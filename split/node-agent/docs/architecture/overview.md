# Architecture overview

RyazanVPN consists of:
- `control-plane` (authoritative state, API, orchestration)
- `node-agent` (runtime controller on each node)
- runtime containers on every node:
  - `amnezia-awg` for Speed (AmneziaWG)
  - `xray` for Health (Reality)

## Runtime control model

`node-agent` is the only runtime controller.

- No desktop Amnezia GUI in production flow.
- No manual peer/client edits in runtime containers.
- control-plane desired-state remains source of truth.

## Node stack

`docker-compose.yml` starts a self-contained stack:
- `node-agent`
- `amnezia-awg`
- `xray`
