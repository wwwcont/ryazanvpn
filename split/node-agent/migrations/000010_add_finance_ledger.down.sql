DROP INDEX IF EXISTS idx_user_ledger_user_created;
DROP TABLE IF EXISTS user_ledger;

ALTER TABLE users
    DROP COLUMN IF EXISTS last_charge_at,
    DROP COLUMN IF EXISTS balance_kopecks;
