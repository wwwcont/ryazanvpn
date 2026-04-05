DROP TRIGGER IF EXISTS trg_audit_logs_updated_at ON audit_logs;
DROP TRIGGER IF EXISTS trg_config_download_tokens_updated_at ON config_download_tokens;
DROP TRIGGER IF EXISTS trg_node_operations_updated_at ON node_operations;
DROP TRIGGER IF EXISTS trg_device_accesses_updated_at ON device_accesses;
DROP TRIGGER IF EXISTS trg_devices_updated_at ON devices;
DROP TRIGGER IF EXISTS trg_vpn_nodes_updated_at ON vpn_nodes;
DROP TRIGGER IF EXISTS trg_invite_code_activations_updated_at ON invite_code_activations;
DROP TRIGGER IF EXISTS trg_invite_codes_updated_at ON invite_codes;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS config_download_tokens;
DROP TABLE IF EXISTS node_operations;
DROP TABLE IF EXISTS device_accesses;
DROP TABLE IF EXISTS devices;
DROP TABLE IF EXISTS vpn_nodes;
DROP TABLE IF EXISTS invite_code_activations;
DROP TABLE IF EXISTS invite_codes;
DROP TABLE IF EXISTS users;

DROP FUNCTION IF EXISTS set_updated_at;
