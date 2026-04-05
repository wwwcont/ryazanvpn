INSERT INTO invite_codes (code, status, max_activations)
VALUES
    ('1111', 'active', 1),
    ('2222', 'active', 1),
    ('3333', 'active', 1)
ON CONFLICT (code) DO NOTHING;

-- Keep seed compatible with the initial schema. New runtime endpoint fields
-- are added and backfilled in later migrations.
INSERT INTO vpn_nodes (name, region, endpoint, status)
VALUES
    ('mvp-node-1', 'single-server', '193.29.224.182:41475', 'active')
ON CONFLICT (name) DO NOTHING;
