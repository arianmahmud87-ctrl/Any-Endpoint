# Server A deployment

This deployment uses one public HTTPS origin for the Vite frontend, management API, and `/v1` gateway. PostgreSQL, Redis, and the worker-control listener stay private.

## Before deployment

1. Install Docker Engine and the Compose plugin on VPS A.
2. Copy `.env.server-a.example` to `.env` and inject real values out-of-band. Never commit `.env`.
3. Replace `anyendpoint.online` in `infra/Caddyfile.server-a.example` and `PUBLIC_BASE_URL` with the temporary `nip.io` hostname or final domain.
4. Add the exact Google OAuth callback:

   `https://anyendpoint.online/api/auth/google/callback`

5. Place the private CA/server/client certificates under `infra/secrets/` with mode `0600`. The control-plane container reads them at `/run/secrets`.
6. Ensure VPS B is reachable through the WireGuard/private address in `WORKER_ALLOWED_HOSTS`; set `WIREGUARD_CONTROL_PLANE_IP` to VPS A's WireGuard address. Compose publishes port 8081 only on that WireGuard interface; do not expose it on the public interface.

## Build and start

From `ai-control-plane`:

```bash
cp .env.server-a.example .env
# Edit .env using the server's secret injection procedure.
# Edit infra/Caddyfile.server-a.example with the real hostname.
docker compose --env-file .env -f infra/docker-compose.server-a.yml build
```

The frontend image always builds with `VITE_PREVIEW_MODE=false`. The frontend build is copied into a private named volume and served read-only by Caddy.

## Database migration gate

The PostgreSQL init-directory mount applies SQL only when the PostgreSQL volume is created for the first time. It does not upgrade an existing database. Before an upgrade:

- take an encrypted PostgreSQL backup;
- review the migration files in lexical order;
- apply them with the approved migration procedure;
- verify `/readyz` and application smoke tests;
- be especially careful with `000006_remove_audit_log.sql`, which is destructive.

Do not treat `docker compose up` as an upgrade migration mechanism.

## Start and verify

```bash
docker compose --env-file .env -f infra/docker-compose.server-a.yml up -d postgres redis
# Apply/review migrations before starting the application on an existing volume.
docker compose --env-file .env -f infra/docker-compose.server-a.yml up -d frontend-build control-plane caddy
curl --fail --silent --show-error https://anyendpoint.online/healthz
curl --fail --silent --show-error https://anyendpoint.online/readyz
```

The public firewall should allow only TCP 80/443 and restricted SSH. PostgreSQL, Redis, and 8080 must not be published. Port 8081 may be reachable only on the WireGuard interface from VPS B and must be protected by mTLS plus host firewall rules. Provider credentials stay on VPS B and are never uploaded through the browser or public API.

## Rollback

Keep the previous image/repository revision available. Restore the encrypted database backup before rolling back across a destructive or incompatible migration. Revoke/re-enroll worker credentials after a control-plane identity or certificate change.
