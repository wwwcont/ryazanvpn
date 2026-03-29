# Runbook: rotate node-agent HMAC secret

1. Generate new secret.
2. Update `NODE_AGENT_HMAC_SECRET` on backend and `AGENT_HMAC_SECRET` on node(s).
3. Roll restart node-agent(s), then control-plane.
4. Verify traffic/health workers stop returning signature errors.
