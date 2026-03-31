WITH TargetClient AS (
    -- Use a single place to define your parameter for easier maintenance
    SELECT {:client_name} AS name
),

SELECT * daily_transactions
WHERE client_name = (SELECT name FROM TargetClient)
