CREATE TABLE IF NOT EXISTS access_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    invite_code_id UUID NOT NULL REFERENCES invite_codes (id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('active', 'expired', 'revoked')),
    activated_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    devices_limit INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_access_grants_user_id
    ON access_grants (user_id);

CREATE INDEX IF NOT EXISTS idx_access_grants_status
    ON access_grants (status);

CREATE INDEX IF NOT EXISTS idx_access_grants_expires_at
    ON access_grants (expires_at);

CREATE TRIGGER trg_access_grants_updated_at
BEFORE UPDATE ON access_grants
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
