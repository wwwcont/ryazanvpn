# Runbook: node deploy

1. Подготовьте env:
   ```bash
   cp deploy/env/node.env.example .env.node.generated
   ```
2. Проверьте обязательные переменные:
   - `NODE_ID`, `NODE_TOKEN`, `CONTROL_PLANE_BASE_URL`
   - `AGENT_HMAC_SECRET`
   - `AMNEZIA_CONTAINER_NAME`, `AMNEZIA_INTERFACE_NAME`, `XRAY_CONTAINER_NAME`
3. Запустите стек:
   ```bash
   docker compose --env-file .env.node.generated -f docker-compose.node.yml up -d --build
   ```
4. Проверьте состояние:
   ```bash
   docker compose --env-file .env.node.generated -f docker-compose.node.yml ps
   curl -fsS http://localhost:8081/health
   curl -fsS http://localhost:8081/ready
   docker exec amnezia-awg2 awg show awg0
   docker logs ryazanvpn-xray --tail 50
   ```
