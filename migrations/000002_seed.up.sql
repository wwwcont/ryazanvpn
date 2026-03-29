INSERT INTO invite_codes (code, status, max_activations)
VALUES
    ('1111', 'active', 1),
    ('2222', 'active', 1),
    ('3333', 'active', 1)
ON CONFLICT (code) DO NOTHING;

INSERT INTO vpn_nodes (name, region, endpoint, agent_base_url, vpn_endpoint, vpn_endpoint_host, vpn_endpoint_port, server_public_key, vpn_subnet_cidr, status)
VALUES
    ('mvp-node-1', 'single-server', '193.29.224.182:41475', 'http://node-agent:8081', '193.29.224.182:41475', '193.29.224.182', 41475, 'iyuNicNyxL3EWzP3JgRJdKozE8TXOArEU6TGcMoK5CU=', '10.8.1.0/24', 'active')
ON CONFLICT (name) DO NOTHING;
