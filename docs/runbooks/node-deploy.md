# Runbook: node deploy

1. Fill `.env.node.generated` from `deploy/env/node.env.example`.
2. Ensure Amnezia runtime container and interface are correct.
3. Run `make run-node`.
4. Verify `GET /health` and `GET /ready` on node-agent.
