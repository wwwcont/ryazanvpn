ALTER TABLE users
    ADD COLUMN IF NOT EXISTS balance_kopecks BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_charge_at DATE;

CREATE TABLE IF NOT EXISTS user_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL CHECK (operation_type IN ('invite_bonus', 'daily_charge', 'payment', 'manual_adjustment')),
    amount_kopecks BIGINT NOT NULL,
    balance_after_kopecks BIGINT NOT NULL,
    reference TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_ledger_user_created
    ON user_ledger (user_id, created_at DESC);
