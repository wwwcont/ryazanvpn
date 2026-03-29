# Deploy guide: backend server (control-plane)

## 1) Build binaries

```bash
GOOS=linux GOARCH=amd64 go build -o /opt/ryazanvpn/bin/control-plane ./cmd/control-plane
GOOS=linux GOARCH=amd64 go build -o /opt/ryazanvpn/bin/node-agent ./cmd/node-agent
```

## 2) Configure env for control-plane

Create `/etc/ryazanvpn/control-plane.env`:

```env
HTTP_ADDR=:8080
POSTGRES_URL=postgres://vpn:vpn@127.0.0.1:5432/vpn?sslmode=disable
REDIS_ADDR=127.0.0.1:6379
LOG_LEVEL=info
ADMIN_API_SECRET=change-me
ADMIN_API_SECRET_HEADER=X-Admin-Secret
NODE_HEALTH_POLL_INTERVAL=15s
NODE_HEALTH_CHECK_TIMEOUT=3s
CONFIG_MASTER_KEY=<base64-key>
VPN_SUBNET_CIDR=10.8.1.0/24
VPN_SERVER_PUBLIC_ENDPOINT=193.29.224.182:41475
VPN_SERVER_PUBLIC_KEY=iyuNicNyxL3EWzP3JgRJdKozE8TXOArEU6TGcMoK5CU=
VPN_CLIENT_ALLOWED_IPS=0.0.0.0/0,::/0
```

## 3) Install systemd unit

```bash
cp deploy/systemd/ryazanvpn-control-plane.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now ryazanvpn-control-plane.service
```

## 4) Smoke checks

```bash
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:8080/ready
```

## Notes

- Admin endpoints require `ADMIN_API_SECRET` header authentication.
- For production, place control-plane behind reverse proxy/TLS.
