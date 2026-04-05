# mTLS design for node ↔ control-plane channel

## Goal
- Replace shared-node-token authentication with per-node client certificates.
- Keep HMAC signatures as transitional defense while mTLS rollout is in progress.

## Identity model
- Each node receives a unique client certificate (`CN=node_id`, SAN URI `spiffe://ryazanvpn/nodes/<node_id>`).
- Control-plane exposes HTTPS with server cert trusted by node agents.
- Control-plane verifies:
  - certificate chain (cluster CA),
  - certificate revocation status,
  - that certificate identity maps to the same `X-Node-Id` header.

## Certificate lifecycle
1. **Issue**  
   - Bootstrap token is exchanged once for a short-lived CSR authorization.
   - Node agent generates private key locally and submits CSR.
   - Control-plane signer issues cert with short TTL (for example, 24h).
2. **Rotate**  
   - Node agent renews before expiry (for example at 70% lifetime).
   - Both old and new certs are accepted during overlap window.
3. **Revoke**  
   - Compromised node cert is added to denylist (Redis + persistent DB table).
   - Control-plane rejects revoked cert serial immediately.

## Rollout without downtime
1. **Phase 1 (current)**: HMAC + `X-Node-Id` + anti-replay.
2. **Phase 2**: Enable optional client cert verification in control-plane ingress.
3. **Phase 3**: Agents start presenting client certs; control-plane logs mTLS identity but still accepts HMAC-only.
4. **Phase 4**: Require mTLS for `/nodes/*`, keep HMAC as second factor for a limited transition period.
5. **Phase 5**: Remove shared registration token from operational path.

## Operational notes
- Store CA root/intermediate in secrets manager.
- Persist issuance + revocation audit events.
- Alert on:
  - near-expiration certs,
  - repeated failed mTLS handshakes,
  - cert subject/header node mismatch.
