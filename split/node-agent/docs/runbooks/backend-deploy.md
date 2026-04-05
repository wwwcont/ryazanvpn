# Runbook: backend deploy

1. Fill `.env.backend.generated` from `deploy/env/backend.env.example`.
2. Start backend stack: `make run-backend`.
3. Register each node with `agent_base_url` and VPN endpoint fields in `vpn_nodes`.
