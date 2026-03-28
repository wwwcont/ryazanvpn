DROP TRIGGER IF EXISTS trg_access_grants_updated_at ON access_grants;

DROP INDEX IF EXISTS idx_access_grants_expires_at;
DROP INDEX IF EXISTS idx_access_grants_status;
DROP INDEX IF EXISTS idx_access_grants_user_id;

DROP TABLE IF EXISTS access_grants;
