# Backend deploy (scalable mode)

## Stack mode

Use `docker-compose.backend.yml` for centralized backend:

- postgres
- redis
- control-plane
- caddy

## Quick start

```bash
cp deploy/env/backend.env.example .env
./scripts/bootstrap-backend.sh
```

## Required env

- `POSTGRES_URL`
- `REDIS_*`
- `AGENT_HMAC_SECRET` / `NODE_AGENT_HMAC_SECRET`
- `NODE_REGISTRATION_TOKEN` (used by node bootstrap/register)

## Notes

- `control-plane` exposes node-management endpoints:
  - `POST /nodes/register`
  - `POST /nodes/heartbeat`
  - `GET /nodes/desired-state`
  - `POST /nodes/apply`
- Node registration requires signed request + `node_token`.
