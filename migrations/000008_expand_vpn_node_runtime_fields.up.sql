ALTER TABLE vpn_nodes
    ADD COLUMN IF NOT EXISTS vpn_endpoint_host TEXT,
    ADD COLUMN IF NOT EXISTS vpn_endpoint_port INTEGER,
    ADD COLUMN IF NOT EXISTS server_public_key TEXT,
    ADD COLUMN IF NOT EXISTS vpn_subnet_cidr TEXT,
    ADD COLUMN IF NOT EXISTS runtime_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE vpn_nodes
SET vpn_endpoint_host = split_part(vpn_endpoint, ':', 1),
    vpn_endpoint_port = NULLIF(split_part(vpn_endpoint, ':', 2), '')::INTEGER
WHERE (vpn_endpoint_host IS NULL OR btrim(vpn_endpoint_host) = '')
  AND position(':' in vpn_endpoint) > 0;

UPDATE vpn_nodes
SET vpn_endpoint_host = vpn_endpoint
WHERE (vpn_endpoint_host IS NULL OR btrim(vpn_endpoint_host) = '')
  AND position(':' in vpn_endpoint) = 0;

UPDATE vpn_nodes
SET vpn_endpoint_port = 41475
WHERE vpn_endpoint_port IS NULL;

UPDATE vpn_nodes
SET server_public_key = COALESCE(NULLIF(server_public_key, ''), 'iyuNicNyxL3EWzP3JgRJdKozE8TXOArEU6TGcMoK5CU='),
    vpn_subnet_cidr = COALESCE(NULLIF(vpn_subnet_cidr, ''), '10.8.1.0/24');

ALTER TABLE vpn_nodes
    ALTER COLUMN vpn_endpoint_host SET NOT NULL,
    ALTER COLUMN vpn_endpoint_port SET NOT NULL,
    ALTER COLUMN server_public_key SET NOT NULL,
    ALTER COLUMN vpn_subnet_cidr SET NOT NULL;
