# Monorepo split plan (day-2 ready)

## Target structure

- `services/control-plane/*`
- `services/node-agent/*`
- `shared/contracts/*`

## Current readiness delivered

1. Per-service build/test/deploy entrypoints:
   - `services/control-plane/Makefile`
   - `services/node-agent/Makefile`
2. Shared versioned contracts:
   - `shared/contracts/nodeapi/*`
   - protocol header versioning (`X-Protocol-Version`)
3. CI matrix split by service and separate contract/boundary checks:
   - `.github/workflows/ci-services.yml`
4. Dependency boundary guard:
   - `scripts/check-dependency-boundaries.sh`

## Split execution plan

### Phase 1 (pre-split hardening)
- Keep monorepo, enforce service-specific CI and contracts.
- Freeze new cross-service imports except via `shared/contracts`.

### Phase 2 (repo extraction)
- Extract `services/control-plane` to new repository.
- Extract `services/node-agent` to new repository.
- Move `shared/contracts` into dedicated contracts module (or vendored submodule).

### Phase 3 (post-split compatibility)
- Contract tests run cross-repo in CI.
- Release train:
  - control-plane validates backward compatibility for previous protocol version,
  - node-agent rollout canary before full fleet update.

## Risks

- Hidden import couplings from `internal/*` packages.
- Contract drift without versioned compatibility tests.
- Operational drift in deployment scripts after split.

## Mitigations

- Boundary checks in CI.
- Protocol version header validation in control-plane.
- Per-service Makefiles and deploy targets retained before split.
