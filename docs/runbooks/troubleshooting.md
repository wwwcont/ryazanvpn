# Troubleshooting runbook

## Node health is degraded

1. `docker ps` and confirm `amnezia-awg`, `xray`, `node-agent` are running.
2. Check node-agent logs for runtime health errors.
3. Verify docker socket mount and container names from `.env.node.generated`.

## Xray runtime unavailable

1. Validate `XRAY_CONTAINER_NAME` matches compose container name.
2. Check `deploy/node/xray/config.json` validity.
3. Restart xray service and verify node-agent heartbeat protocols.

## Unexpected runtime drift

1. Avoid manual runtime edits.
2. Wait reconcile interval.
3. Verify `/nodes/desired-state` and `/nodes/apply` reports.
