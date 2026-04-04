ALTER TABLE device_accesses
    ADD COLUMN IF NOT EXISTS preshared_key TEXT;
