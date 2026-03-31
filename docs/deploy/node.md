# Node deploy (scalable mode)

## Stack mode

Use `docker-compose.node.yml` on each VPN node host.

## Quick start

```bash
cp deploy/env/node.env.example .env.node.generated
# fill NODE_ID, NODE_TOKEN, CONTROL_PLANE_BASE_URL, AGENT_HMAC_SECRET
./scripts/bootstrap-node.sh
```

## Required node identity

- `NODE_ID` — UUID of `vpn_nodes.id` from control-plane DB.
- `NODE_TOKEN` — shared registration token.

Node-agent startup flow:

1. `POST /nodes/register`
2. periodic `POST /nodes/heartbeat` (30–60s)
3. `GET /nodes/desired-state`
4. reconcile runtime and `POST /nodes/apply`

This enables automatic node attachment after startup.
