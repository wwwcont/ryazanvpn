ALTER TABLE vpn_nodes
    ADD COLUMN IF NOT EXISTS endpoint TEXT;

UPDATE vpn_nodes
SET endpoint = COALESCE(NULLIF(btrim(vpn_endpoint), ''), endpoint);

ALTER TABLE vpn_nodes
    ALTER COLUMN endpoint SET NOT NULL;

ALTER TABLE vpn_nodes
    DROP COLUMN IF EXISTS agent_base_url,
    DROP COLUMN IF EXISTS vpn_endpoint;
