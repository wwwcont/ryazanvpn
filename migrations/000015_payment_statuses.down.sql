ALTER TABLE users
    ALTER COLUMN status DROP DEFAULT;
ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users
    ADD CONSTRAINT users_status_check CHECK (status IN ('active', 'blocked'));
ALTER TABLE users
    ALTER COLUMN status SET DEFAULT 'active';

ALTER TABLE device_accesses
    DROP CONSTRAINT IF EXISTS device_accesses_status_check;
ALTER TABLE device_accesses
    ADD CONSTRAINT device_accesses_status_check CHECK (status IN ('pending', 'active', 'suspended', 'revoked', 'error'));

ALTER TABLE user_ledger
    DROP CONSTRAINT IF EXISTS user_ledger_operation_type_check;
ALTER TABLE user_ledger
    ADD CONSTRAINT user_ledger_operation_type_check CHECK (operation_type IN ('invite_bonus', 'daily_charge', 'payment', 'manual_adjustment'));
