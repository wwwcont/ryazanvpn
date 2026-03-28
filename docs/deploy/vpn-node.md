# Deploy guide: VPN node (node-agent)

## 1) Build/install node-agent binary

```bash
GOOS=linux GOARCH=amd64 go build -o /opt/ryazanvpn/bin/node-agent ./cmd/node-agent
```

## 2) Configure env for node-agent

Create `/etc/ryazanvpn/node-agent.env`:

```env
HTTP_ADDR=:8081
LOG_LEVEL=info
AGENT_HMAC_SECRET=change-me
RUNTIME_ADAPTER=mock
RUNTIME_WORK_DIR=/var/lib/ryazanvpn/node-agent
AWG_BINARY_PATH=/usr/bin/awg
WG_BINARY_PATH=/usr/bin/wg
IP_BINARY_PATH=/usr/sbin/ip
RUNTIME_EXEC_TIMEOUT=10s
```

`node-agent` не требует PostgreSQL/Redis для startup, `/health`, `/ready` и операций apply/revoke.

## 3) Install systemd unit

```bash
cp deploy/systemd/ryazanvpn-node-agent.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now ryazanvpn-node-agent.service
```

## 4) Smoke checks

```bash
curl -sS http://127.0.0.1:8081/health
curl -sS http://127.0.0.1:8081/ready
```

## Runtime adapters

- `RUNTIME_ADAPTER=mock` — default безопасный режим для MVP.
- `RUNTIME_ADAPTER=shell` — подготовленный шаблон shell-runtime adapter.
  - apply/revoke команды **намеренно не реализованы** (TODO) до появления проверенных командных последовательностей управления AWG/WG.
