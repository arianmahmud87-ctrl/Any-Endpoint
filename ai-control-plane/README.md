# AI Control Plane

Secure, backend-first control plane for the public Codex/Claude Code gateway.

> **Status:** Public multi-user foundation. Google OIDC, server-side sessions, CSRF, organization authorization, provider profile registry, scoped API-key lifecycle, private worker enrollment, mTLS, public worker routing, provider login-attempt state APIs, usage recording, and same-origin frontend deployment are implemented. The worker command channel, Codex device-code runner, per-profile worker provisioning, quotas/billing, and customer-owned worker app remain staged work.

## Project structure

```text
ai-control-plane/
├─ backend/       Go API service and security middleware
├─ db/migrations/ PostgreSQL schema migrations
├─ infra/         Production Docker Compose, Dockerfile, Caddy configuration
├─ frontend/      Same-origin React/TypeScript dashboard
├─ worker-agent/  Profile-bound worker runtime and mTLS client
└─ docs/          Threat model, security baseline, migration and worker protocols
```

## Run locally

From `ai-control-plane`:

```powershell
$env:APP_ENV = "development"
go run ./backend/cmd/control-plane
```

For local development, run the Go service directly with development dependencies supplied separately. The production deployment is the supported Compose stack described in `docs/SERVER_A_DEPLOYMENT.md`.

## Endpoints

- `GET /healthz` — process liveness; no dependency details
- `GET /readyz` — dependency and authentication readiness
- `GET /api/auth/google/start` — starts Google authorization-code + PKCE login
- `GET /api/auth/google/callback` — validates state, nonce, PKCE and Google ID token
- `GET /api/me` — authenticated user/org information and CSRF token
- `GET /api/profiles` — authenticated organization-scoped profile list
- `POST /api/profiles` — owner/admin creates a Codex or Claude Code profile
- `GET /api/profiles/{id}` — authenticated organization-scoped profile detail
- `POST /api/profiles/{id}/login/start` — owner/admin starts a provider login attempt
- `GET /api/profiles/{id}/login/status?attempt_id=...` — reads a scoped login attempt state
- `POST /api/profiles/{id}/login/cancel?attempt_id=...` — cancels an active provider login attempt
- `POST /api/profiles/{id}/login/retry?attempt_id=...` — cancels and starts a replacement attempt
- `POST /api/profiles/{id}/disable` — revokes worker transport and disables profile
- `POST /api/profiles/{id}/reconnect` — drains old worker and creates a new enrollment attempt
- `GET /api/keys` — authenticated key metadata; raw keys are never returned
- `POST /api/keys` — creates a Codex, Claude, or universal key and reveals it once
- `POST /api/keys/{id}/rotate` — creates a replacement and revokes the old key
- `POST /api/keys/{id}/revoke` — immediately revokes a key
- `GET /api/usage` — tenant-scoped request/latency records without prompt content
- `GET /api/operations/summary` — profile/key/worker and 24-hour error summary
- `POST /api/playground/runs` — session-authorized profile/model prompt execution
- `POST /api/auth/logout` — authenticated, CSRF-protected session revocation

The worker enrollment and heartbeat routes listen only on `INTERNAL_HTTP_ADDR` (default `127.0.0.1:8081`), are not routed by Caddy, and are not part of the public API:

- `POST /internal/worker/enroll` — consumes `{token, agent_id, internal_url}` once and returns an encrypted-transport-backed worker token
- `POST /internal/worker/heartbeat`

Phase 3 never accepts provider passwords, bearer tokens, `auth.json`, Claude credential files, or raw secret material through the browser or public API. Profile creation creates only an opaque secret-manager reference. A worker receives a one-time enrollment token and stores its provider credential in its own isolated runtime.


## Authentication and profile connection setup

Create a Google OAuth web application with this exact redirect URI:

```text
http://127.0.0.1:8080/api/auth/google/callback
```

Then inject the values at runtime; never commit the client secret:

```powershell
$env:AUTH_ENABLED = "true"
$env:GOOGLE_CLIENT_ID = "your-client-id.apps.googleusercontent.com"
$env:GOOGLE_CLIENT_SECRET = "injected-secret"
$env:SESSION_SECRET = "at-least-32-random-bytes"
$env:KEY_PEPPER = "at-least-32-random-bytes"
go run ./backend/cmd/control-plane
```

The production deployment uses `infra/docker-compose.server-a.yml` and `infra/Caddyfile.server-a.example`; the live stack serves the frontend and API from the same origin. Apply database migrations separately before upgrades; the PostgreSQL init directory is only for a fresh volume.

## Production guardrails

`APP_ENV=production` refuses to start unless all of these are present:

- `PUBLIC_BASE_URL` uses `https://` and has no query/fragment;
- `TRUST_PROXY` is explicitly configured;
- `SESSION_SECRET` and `KEY_PEPPER` are at least 32 bytes;
- `SECRET_BACKEND` is not `local`;
- `AUTH_ENABLED=true`;
- Google issuer, client ID and runtime-injected client secret;
- PostgreSQL and Redis URLs, reachable before listener bind;
- Phase 2, Phase 3, and provider-login migrations applied;

Provider credentials and public API keys are still intentionally absent. Do not place Codex auth files, Claude credentials, Google tokens, or real secrets in this repository.


## Phase 4 API keys

Create a key through the authenticated management API. The request must include a CSRF token from `/api/me`:

```json
{
  "name": "my-codex-client",
  "type": "codex",
  "expires_at": "2027-01-01T00:00:00Z",
  "grants": [
    {
      "profile_id": "profile-uuid",
      "model_pattern": "*",
      "routes": ["models", "responses", "chat.completions"],
      "requests_per_minute": 60
    }
  ]
}
```

Supported key types are `codex`, `claude`, and `universal`. A universal key is not unrestricted: every provider/profile/route grant is explicit. The generated `skv1_...` secret is returned only on create or rotate and is never recoverable afterward. PostgreSQL stores only its prefix and a peppered HMAC verifier.

The public gateway now verifies the key, selects exactly one grant, applies the Redis rate limit and server concurrency cap, checks a live private worker, and forwards only an allowlisted request header set. Provider worker responses are bounded and normalized on transport failure. The user key is never forwarded; the worker transport token is decrypted only in memory for the outbound request.


## Provider worker and mTLS boundary

The control plane now has an mTLS configuration boundary for the private listener and outbound worker requests. Enable it in production with:

```text
MTLS_ENABLED=true
MTLS_CA_FILE=/run/secrets/worker-ca.pem
MTLS_SERVER_CERT_FILE=/run/secrets/control-plane-server.pem
MTLS_SERVER_KEY_FILE=/run/secrets/control-plane-server-key.pem
MTLS_CLIENT_CERT_FILE=/run/secrets/control-plane-client.pem
MTLS_CLIENT_KEY_FILE=/run/secrets/control-plane-client-key.pem
```

Certificates must be issued by a private CA, use SANs matching the internal worker hostname, and be rotated through a controlled restart/rolling deployment. The public Caddy listener remains the public TLS boundary; the internal listener requires verified client certificates when mTLS is enabled.

`worker-agent/` contains a profile-bound enrollment/heartbeat client and provider adapter interfaces for Codex and Claude Code. The concrete provider adapters intentionally return `not implemented` until isolated Codex `auth.json` and Claude CLI credential/home strategies are verified. Do not run the scaffold as a production provider worker yet.


## Secure playground

`POST /api/playground/runs` is a browser-session endpoint, not a public-key endpoint. It accepts only `profile_id`, `model`, `prompt`, and optional `stream`; the backend checks organization ownership, profile status, allowed model scope, CSRF, per-user rate limit, concurrency, and worker health. It forwards the request through the private worker transport and records usage without storing prompt or response content. The frontend never receives provider credentials or needs a durable API key for this action.
