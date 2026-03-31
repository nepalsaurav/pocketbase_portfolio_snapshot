WITH TargetClient AS (
    -- Use a single place to define your parameter for easier maintenance
    SELECT {:client_name} AS name
),

BaseHoldings AS (
    SELECT
        client_name,
        symbol,
        -- Net base quantity (Buys - Sells)
        SUM(CASE WHEN trn_type = 'buy' THEN qty ELSE -qty END) AS base_qty,
        -- Total historical buys (used for WACC denominator)
        SUM(CASE WHEN trn_type = 'buy' THEN qty ELSE 0 END) AS total_buy_qty,
        -- Total historical buy cost (used for WACC numerator)
        SUM(CASE WHEN trn_type = 'buy'
                 THEN (qty * rate) + broker_commission + nepse_commission + sebo_commission + dp_charge
                 ELSE 0 END) AS total_buy_cost
    FROM daily_transactions
    WHERE client_name = (SELECT name FROM TargetClient)
    GROUP BY symbol
),

ActionDates AS (
    SELECT id, symbol, book_close_date, bonus_pct AS val, 'bonus' AS type, listing_date
    FROM corporate_actions_bonus
    WHERE symbol IN (SELECT symbol FROM BaseHoldings)

    UNION ALL

    SELECT id, symbol, book_close_date,
           (CAST(SUBSTR(right_share_ratio, INSTR(right_share_ratio, ':') + 1) AS REAL) /
            CAST(SUBSTR(right_share_ratio, 1, INSTR(right_share_ratio, ':') - 1) AS REAL)) AS val,
           'right' AS type, listing_date
    FROM corporate_actions_right_share
    WHERE symbol IN (SELECT symbol FROM BaseHoldings)

    UNION ALL

    SELECT id, symbol, book_close_date, cash_dividend_pct AS val, 'dividend' AS type, NULL AS listing_date
    FROM corporate_actions_cash_dividend
    WHERE symbol IN (SELECT symbol FROM BaseHoldings)
),

ActionEligibility AS (
    SELECT
        (SELECT name FROM TargetClient) AS client_name,
        a.symbol,
        a.id AS action_id,
        a.type,
        a.val,
        a.listing_date,

        MAX(0,
            COALESCE((
                SELECT SUM(CASE WHEN trn_type = 'buy' THEN qty ELSE -qty END)
                FROM daily_transactions
                WHERE symbol = a.symbol
                  AND client_name = (SELECT name FROM TargetClient)
                  AND date < a.book_close_date
            ), 0)
            +
            COALESCE((
                SELECT SUM(
                    ROUND(
                        COALESCE((
                            SELECT SUM(CASE WHEN trn_type = 'buy' THEN qty ELSE -qty END)
                            FROM daily_transactions
                            WHERE symbol = prev_b.symbol
                              AND client_name = (SELECT name FROM TargetClient)
                              AND date < prev_b.book_close_date
                        ), 0) * (prev_b.bonus_pct / 100.0)
                    , 0)
                )
                FROM corporate_actions_bonus prev_b
                WHERE prev_b.symbol = a.symbol
                  AND prev_b.book_close_date < a.book_close_date
            ), 0)
            +
            COALESCE((
                SELECT SUM(
                    ROUND(
                        COALESCE((
                            SELECT SUM(CASE WHEN trn_type = 'buy' THEN qty ELSE -qty END)
                            FROM daily_transactions
                            WHERE symbol = prev_r.symbol
                              AND client_name = (SELECT name FROM TargetClient)
                              AND date < prev_r.book_close_date
                        ), 0) * (CAST(SUBSTR(prev_r.right_share_ratio, INSTR(prev_r.right_share_ratio, ':') + 1) AS REAL) /
                         CAST(SUBSTR(prev_r.right_share_ratio, 1, INSTR(prev_r.right_share_ratio, ':') - 1) AS REAL))
                    , 0)
                )
                FROM corporate_actions_right_share prev_r
                WHERE prev_r.symbol = a.symbol
                  AND prev_r.book_close_date < a.book_close_date
            ), 0)
        ) AS eligible_qty

    FROM ActionDates a
),

CorporateCalculations AS (
    SELECT
        symbol,
        SUM(CASE WHEN type = 'bonus' AND (listing_date IS NULL OR listing_date = '') THEN ROUND(eligible_qty * (val / 100.0), 0) ELSE 0 END) AS prov_bonus,
        SUM(CASE WHEN type = 'bonus' AND listing_date != '' THEN ROUND(eligible_qty * (val / 100.0), 0) ELSE 0 END) AS act_bonus,
        SUM(CASE WHEN type = 'right' AND (listing_date IS NULL OR listing_date = '') THEN ROUND(eligible_qty * val, 0) ELSE 0 END) AS prov_right,
        SUM(CASE WHEN type = 'right' AND listing_date != '' THEN ROUND(eligible_qty * val, 0) ELSE 0 END) AS act_right,
        SUM(CASE WHEN type = 'dividend' THEN eligible_qty * val ELSE 0 END) AS total_cash_dividend
    FROM ActionEligibility
    GROUP BY symbol
),

FinalPortfolio AS (
    SELECT
        bh.client_name,
        bh.symbol,

        MAX(0, bh.base_qty) AS traded_qty,
        COALESCE(cc.prov_bonus, 0) AS provisional_bonus_shares,
        COALESCE(cc.act_bonus, 0) AS actual_bonus_shares,
        COALESCE(cc.prov_right, 0) AS provisional_right_shares,
        COALESCE(cc.act_right, 0) AS actual_right_shares,
        COALESCE(cc.total_cash_dividend, 0) AS total_cash_dividend,

        (MAX(0, bh.base_qty) + COALESCE(cc.prov_bonus, 0) + COALESCE(cc.act_bonus, 0) +
         COALESCE(cc.prov_right, 0) + COALESCE(cc.act_right, 0)) AS current_holding_qty,

        -- THE FIX: Bonus Shares * 100.0 is now added to the WACC Numerator
        (bh.total_buy_cost +
         ((COALESCE(cc.prov_right, 0) + COALESCE(cc.act_right, 0)) * 100.0) +
         ((COALESCE(cc.prov_bonus, 0) + COALESCE(cc.act_bonus, 0)) * 100.0)
        ) /
        NULLIF(bh.total_buy_qty + COALESCE(cc.prov_bonus, 0) + COALESCE(cc.act_bonus, 0) +
               COALESCE(cc.prov_right, 0) + COALESCE(cc.act_right, 0), 0) AS average_cost

    FROM BaseHoldings bh
    LEFT JOIN CorporateCalculations cc USING (symbol)
)

SELECT * FROM FinalPortfolio WHERE current_holding_qty > 0;
