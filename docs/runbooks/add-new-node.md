# Runbook: add new node

1. Deploy `node-agent` on new host.
2. Insert row to `vpn_nodes` with fields: `agent_base_url`, `vpn_endpoint_host`, `vpn_endpoint_port`, `server_public_key`, `vpn_subnet_cidr`, `status=active`.
3. Validate via `GET /api/v1/metrics/nodes` and Telegram admin `Статус нод`.
