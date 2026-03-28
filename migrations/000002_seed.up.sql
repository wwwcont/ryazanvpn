INSERT INTO invite_codes (code, status, max_activations)
VALUES
    ('1111', 'active', 1),
    ('2222', 'active', 1),
    ('3333', 'active', 1)
ON CONFLICT (code) DO NOTHING;

INSERT INTO vpn_nodes (name, region, endpoint, status)
VALUES
    ('mvp-node-1', 'single-server', 'SERVER_IP:51820', 'active')
ON CONFLICT (name) DO NOTHING;
