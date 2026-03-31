WITH client_bounds AS (
    SELECT MIN(date) as start_date, MAX(date) as end_date
    FROM daily_transactions
    WHERE client_name = {:client_name}
)

-- 1. Transactions
SELECT
    id, symbol, date AS event_date, 'Transaction' AS source, 'daily_transactions' AS collection_name, created, updated,
    json_object(
        'trn_no', trn_no, 'client_name', client_name, 'trn_type', trn_type,
        'qty', qty, 'rate', rate, 'broker_commission', broker_commission,
        'nepse_commission', nepse_commission, 'sebo_commission', sebo_commission, 'dp_charge', dp_charge
    ) AS metadata
FROM daily_transactions
WHERE client_name = {:client_name}

UNION ALL

-- 2. Bonus
SELECT
    id, symbol, book_close_date AS event_date, 'Bonus' AS source, 'corporate_actions_bonus' AS collection_name, created, updated,
    json_object('bonus_pct', bonus_pct, 'listing_date', listing_date) AS metadata
FROM corporate_actions_bonus, client_bounds
WHERE book_close_date BETWEEN client_bounds.start_date AND client_bounds.end_date

UNION ALL

-- 3. Dividend
SELECT
    id, symbol, book_close_date AS event_date, 'Dividend' AS source, 'corporate_actions_cash_dividend' AS collection_name, created, updated,
    json_object('cash_dividend_pct', cash_dividend_pct, 'listing_date', listing_date) AS metadata
FROM corporate_actions_cash_dividend, client_bounds
WHERE book_close_date BETWEEN client_bounds.start_date AND client_bounds.end_date

UNION ALL

-- 4. Right Share
SELECT
    id, symbol, book_close_date AS event_date, 'Right Share' AS source, 'corporate_actions_right_share' AS collection_name, created, updated,
    json_object('right_share_ratio', right_share_ratio, 'listing_date', listing_date) AS metadata
FROM corporate_actions_right_share, client_bounds
WHERE book_close_date BETWEEN client_bounds.start_date AND client_bounds.end_date

ORDER BY event_date ASC;
