# Troubleshooting runbook

## Node is registered but peers are not applied

1. Check `/nodes/desired-state` response includes expected `protocol` and `peer_public_key`.
2. Check node-agent logs for `reconcile apply failed`.
3. Check `node_apply_reports` for failed records.

## Revoke does not remove peer

1. Verify revoke payload has non-empty `peer_public_key`.
2. Verify protocol in payload matches access protocol.
3. Retry operation and inspect latest `node_apply_reports`.

## Runtime drift after restart

1. Wait one reconcile interval.
2. Compare DB active accesses vs runtime counters.
3. If mismatch persists, enable safe reconcile mode and re-run.
