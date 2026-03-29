ALTER TABLE vpn_nodes
    DROP COLUMN IF EXISTS runtime_metadata,
    DROP COLUMN IF EXISTS vpn_subnet_cidr,
    DROP COLUMN IF EXISTS server_public_key,
    DROP COLUMN IF EXISTS vpn_endpoint_port,
    DROP COLUMN IF EXISTS vpn_endpoint_host;
