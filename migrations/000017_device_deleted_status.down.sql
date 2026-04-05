ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_status_check;

ALTER TABLE devices
    ADD CONSTRAINT devices_status_check CHECK (status IN ('active', 'blocked', 'revoked'));
