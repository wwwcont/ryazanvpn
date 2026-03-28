ALTER TABLE vpn_nodes
    ADD COLUMN IF NOT EXISTS agent_base_url TEXT,
    ADD COLUMN IF NOT EXISTS vpn_endpoint TEXT;

UPDATE vpn_nodes
SET agent_base_url = CASE
        WHEN agent_base_url IS NULL OR btrim(agent_base_url) = '' THEN
            CASE
                WHEN endpoint LIKE 'http://%' OR endpoint LIKE 'https://%' THEN endpoint
                ELSE 'http://' || endpoint
            END
        ELSE agent_base_url
    END,
    vpn_endpoint = CASE
        WHEN vpn_endpoint IS NULL OR btrim(vpn_endpoint) = '' THEN endpoint
        ELSE vpn_endpoint
    END;

ALTER TABLE vpn_nodes
    ALTER COLUMN agent_base_url SET NOT NULL,
    ALTER COLUMN vpn_endpoint SET NOT NULL;

DELETE FROM vpn_nodes WHERE name <> 'mvp-node-1';

UPDATE vpn_nodes
SET region = 'single-server',
    agent_base_url = 'http://node-agent:8081',
    vpn_endpoint = 'SERVER_IP:51820',
    status = 'active'
WHERE name = 'mvp-node-1';

ALTER TABLE vpn_nodes
    DROP COLUMN IF EXISTS endpoint;
