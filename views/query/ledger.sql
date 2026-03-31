WITH TargetClient AS (
    SELECT 'LATA CHAUDHARY (LC456959)' as name
),

-- 1. Process Raw Trades
Trades AS (
    SELECT
        client_name,
        symbol,
        date,
        UPPER(trn_type) AS action_type,
        CASE WHEN trn_type = 'buy' THEN qty ELSE -qty END AS qty,
        CASE
            WHEN trn_type = 'buy' THEN (qty * rate) + broker_commission + nepse_commission + sebo_commission + dp_charge
            ELSE 0
        END AS purchase_cost,
        CASE
            WHEN trn_type = 'sell' THEN (qty * rate) - broker_commission - nepse_commission - sebo_commission - dp_charge
            ELSE 0
        END AS sell_proceeds,
        'ACTUAL' AS reference -- All market trades are actual
    FROM daily_transactions
    WHERE client_name = (SELECT name FROM TargetClient)
),

-- 2. Unified Timeline of Corporate Actions (Added listing_date)
ActionTimeline AS (
    SELECT symbol, book_close_date, bonus_pct / 100.0 AS ratio, 'BONUS' AS type, listing_date
    FROM corporate_actions_bonus

    UNION ALL

    SELECT symbol, book_close_date,
           (CAST(SUBSTR(right_share_ratio, INSTR(right_share_ratio, ':') + 1) AS REAL) /
            CAST(SUBSTR(right_share_ratio, 1, INSTR(right_share_ratio, ':') - 1) AS REAL)) AS ratio,
           'RIGHT' AS type, listing_date
    FROM corporate_actions_right_share

    UNION ALL

    SELECT symbol, book_close_date, cash_dividend_pct / 100.0 AS ratio, 'DIVIDEND' AS type, NULL AS listing_date
    FROM corporate_actions_cash_dividend
),

-- 3. Calculate Eligibility
ActionEligibility AS (
    SELECT
        t_client.name as client_name,
        alt.symbol,
        alt.book_close_date,
        alt.type,
        alt.ratio,
        alt.listing_date, -- Pass listing date down
        (
            COALESCE((SELECT SUM(qty) FROM Trades t WHERE t.symbol = alt.symbol AND t.date < alt.book_close_date), 0) +
            COALESCE((
                SELECT SUM(ROUND(
                    (SELECT SUM(t2.qty) FROM Trades t2 WHERE t2.symbol = b.symbol AND t2.date < b.book_close_date) * (b.bonus_pct / 100.0)
                , 0))
                FROM corporate_actions_bonus b
                WHERE b.symbol = alt.symbol AND b.book_close_date < alt.book_close_date
            ), 0) +
            COALESCE((
                SELECT SUM(ROUND(
                    (SELECT SUM(t3.qty) FROM Trades t3 WHERE t3.symbol = r.symbol AND t3.date < r.book_close_date) * (CAST(SUBSTR(r.right_share_ratio, INSTR(r.right_share_ratio, ':') + 1) AS REAL) / CAST(SUBSTR(r.right_share_ratio, 1, INSTR(r.right_share_ratio, ':') - 1) AS REAL))
                , 0))
                FROM corporate_actions_right_share r
                WHERE r.symbol = alt.symbol AND r.book_close_date < alt.book_close_date
            ), 0)
        ) AS eligible_qty
    FROM ActionTimeline alt
    CROSS JOIN TargetClient t_client
    WHERE EXISTS (SELECT 1 FROM Trades t WHERE t.symbol = alt.symbol AND t.date < alt.book_close_date)
),

-- 4. Map Corporate Actions to Ledger Format
CalculatedActions AS (
    SELECT
        client_name,
        symbol,
        book_close_date AS date,
        type AS action_type,
        CASE WHEN type IN ('BONUS', 'RIGHT') THEN ROUND(eligible_qty * ratio, 0) ELSE 0 END AS qty,
        CASE
            WHEN type IN ('BONUS', 'RIGHT') THEN ROUND(eligible_qty * ratio, 0) * 100.0
            ELSE 0
        END AS purchase_cost,
        CASE WHEN type = 'DIVIDEND' THEN eligible_qty * ratio * 100.0 ELSE 0 END AS sell_proceeds,

        -- Determine if Provisional or Actual
        CASE
            WHEN type = 'DIVIDEND' THEN 'ACTUAL'
            WHEN listing_date IS NULL OR listing_date = '' THEN 'PROVISIONAL'
            ELSE 'ACTUAL'
        END AS reference

    FROM ActionEligibility
    WHERE eligible_qty > 0
),

-- 5. Final Unified Ledger
UnifiedLedger AS (
    SELECT client_name, symbol, date, action_type, qty, purchase_cost, sell_proceeds, reference FROM Trades
    UNION ALL
    SELECT client_name, symbol, date, action_type, qty, purchase_cost, sell_proceeds, reference FROM CalculatedActions
),

-- 6. Add Running Quantity
LedgerWithRunningQty AS (
    SELECT
        *,
        SUM(qty) OVER (PARTITION BY symbol ORDER BY date, action_type ASC) as running_qty
    FROM UnifiedLedger
),

-- 7. Flag whenever a new cycle starts
LedgerFlag AS (
    SELECT
        *,
        CASE WHEN LAG(running_qty, 1, 0) OVER (PARTITION BY symbol ORDER BY date, action_type ASC) = 0 THEN 1 ELSE 0 END as reset_flag
    FROM LedgerWithRunningQty
),

-- 8. Create a Cycle ID
LedgerCycle AS (
    SELECT
        *,
        SUM(reset_flag) OVER (PARTITION BY symbol ORDER BY date, action_type ASC) as cycle_id
    FROM LedgerFlag
),

-- 9. Running Totals
LedgerWithMetrics AS (
    SELECT
        *,
        SUM(purchase_cost) OVER (PARTITION BY symbol, cycle_id ORDER BY date, action_type ASC) as cumulative_cost,
        SUM(CASE WHEN action_type IN ('BUY', 'BONUS', 'RIGHT') THEN qty ELSE 0 END)
            OVER (PARTITION BY symbol, cycle_id ORDER BY date, action_type ASC) as total_units_received
    FROM LedgerCycle
)

-- 10. Final Output
SELECT
    symbol,
    date,
    action_type,
    reference, -- <-- Added to final output
    qty,
    running_qty,
    ROUND(purchase_cost, 2) as cash_out,
    ROUND(sell_proceeds, 2) as cash_in,

    CASE
        WHEN action_type IN ('BUY', 'BONUS', 'RIGHT') THEN ROUND(purchase_cost / NULLIF(qty, 0), 4)
        WHEN action_type = 'SELL' THEN ROUND(sell_proceeds / NULLIF(ABS(qty), 0), 4)
        ELSE 0.0000
    END as transaction_average_cost,

    CASE
        WHEN running_qty = 0 THEN 0.0000
        ELSE ROUND(cumulative_cost / NULLIF(total_units_received, 0), 4)
    END as running_wacc

FROM LedgerWithMetrics
ORDER BY symbol, date, action_type ASC;
