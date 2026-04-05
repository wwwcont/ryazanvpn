# Access lifecycle and source of truth

## Core rule

`control-plane` is the **only source of truth** for access state.

- Runtime adapters (`amneziawg`, `xray`) are execution targets only.
- `node-agent` applies state received from control-plane.

## Protocol-safe lifecycle

`user -> device -> protocol access -> peer -> config`

Each `device_access` is protocol-specific and drives desired-state per protocol.
No runtime default protocol is allowed.

## Revoke semantics

Revoke operation payload must include:
- `device_access_id`
- `protocol`
- `peer_public_key`

Revoke is idempotent:
- repeated revoke must be safe;
- missing peer in runtime is treated as converged state.

## Reconciliation contract

- Node-agent fetches desired state and compares with runtime peers.
- Missing desired peer => apply.
- Extra runtime peer => revoke using runtime peer protocol.
- Apply reports are persisted in `node_apply_reports` for auditability.

## Operational invariants

- No protocol hardcode in orchestrator paths.
- Operation status is updated from apply reports (`applied` / `failed`).
