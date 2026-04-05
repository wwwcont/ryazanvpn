CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS
$$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT NOT NULL UNIQUE,
    username TEXT,
    first_name TEXT,
    last_name TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invite_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'exhausted', 'expired', 'revoked')),
    max_activations INTEGER NOT NULL DEFAULT 1 CHECK (max_activations > 0),
    current_activations INTEGER NOT NULL DEFAULT 0 CHECK (current_activations >= 0),
    exhausted_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS invite_code_activations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invite_code_id UUID NOT NULL REFERENCES invite_codes (id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_invite_code_activations_invite_code_user UNIQUE (invite_code_id, user_id)
);

CREATE TABLE IF NOT EXISTS vpn_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    region TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'draining', 'maintenance')),
    current_load INTEGER NOT NULL DEFAULT 0 CHECK (current_load >= 0),
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    vpn_node_id UUID REFERENCES vpn_nodes (id) ON DELETE SET NULL,
    public_key TEXT NOT NULL UNIQUE,
    name TEXT,
    platform TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'blocked', 'revoked')),
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS device_accesses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id UUID NOT NULL REFERENCES devices (id) ON DELETE CASCADE,
    vpn_node_id UUID NOT NULL REFERENCES vpn_nodes (id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'suspended', 'revoked', 'error')),
    assigned_ip INET,
    config_blob_encrypted BYTEA,
    granted_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_device_accesses_device_node UNIQUE (device_id, vpn_node_id)
);

CREATE TABLE IF NOT EXISTS node_operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vpn_node_id UUID NOT NULL REFERENCES vpn_nodes (id) ON DELETE CASCADE,
    operation_type TEXT NOT NULL CHECK (operation_type IN ('create_peer', 'revoke_peer', 'rotate_keys', 'reload_config', 'restart')),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    
    requested_by_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS config_download_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_access_id UUID NOT NULL REFERENCES device_accesses (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'issued' CHECK (status IN ('issued', 'used', 'expired', 'revoked')),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    action TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invite_codes_status_expires_at
    ON invite_codes (status, expires_at);

CREATE INDEX IF NOT EXISTS idx_invite_code_activations_user_id
    ON invite_code_activations (user_id);

CREATE INDEX IF NOT EXISTS idx_invite_code_activations_invite_code_id
    ON invite_code_activations (invite_code_id);

CREATE INDEX IF NOT EXISTS idx_vpn_nodes_status_last_seen_at
    ON vpn_nodes (status, last_seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_devices_user_id_status
    ON devices (user_id, status);

CREATE INDEX IF NOT EXISTS idx_devices_vpn_node_id
    ON devices (vpn_node_id);

CREATE INDEX IF NOT EXISTS idx_device_accesses_device_id_status
    ON device_accesses (device_id, status);

CREATE INDEX IF NOT EXISTS idx_device_accesses_vpn_node_id_status
    ON device_accesses (vpn_node_id, status);

CREATE INDEX IF NOT EXISTS idx_node_operations_vpn_node_id_status_created_at
    ON node_operations (vpn_node_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_config_download_tokens_device_access_status_expires
    ON config_download_tokens (device_access_id, status, expires_at);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_user_id_created_at
    ON audit_logs (actor_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_entity_type_entity_id_created_at
    ON audit_logs (entity_type, entity_id, created_at DESC);

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_invite_codes_updated_at
BEFORE UPDATE ON invite_codes
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_invite_code_activations_updated_at
BEFORE UPDATE ON invite_code_activations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_vpn_nodes_updated_at
BEFORE UPDATE ON vpn_nodes
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_devices_updated_at
BEFORE UPDATE ON devices
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_device_accesses_updated_at
BEFORE UPDATE ON device_accesses
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_node_operations_updated_at
BEFORE UPDATE ON node_operations
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_config_download_tokens_updated_at
BEFORE UPDATE ON config_download_tokens
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_audit_logs_updated_at
BEFORE UPDATE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
