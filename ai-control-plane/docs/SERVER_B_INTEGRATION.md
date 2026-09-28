# Server B worker integration

This runbook is for the Server B worker-agent that integrates with the Server A control plane. Server A owns profile state, login attempts, command durability, and readiness. Server B owns provider process execution and provider credentials.

## Required private transport

Use Server A's private WireGuard address and internal listener, not the public HTTPS origin. Every request requires:

- TLS 1.3 with Server A's private CA for server verification;
- a CA-signed Server B client certificate and private key for mTLS;
- `Authorization: Bearer <profile-bound-worker-token>` for command, event, and heartbeat requests.

The bearer token is issued once by `POST /internal/worker/enroll` after a one-time enrollment token is consumed. Store it only in the isolated worker runtime. Never commit certificates, keys, enrollment tokens, worker tokens, or provider credentials.

## Polling contract

Poll:

```text
GET /internal/worker/commands
```

Expected responses:

- `200` — JSON envelope containing one claimed command;
- `204` — no queued non-expired command; back off before polling again;
- `401` — fail closed and alert/re-enroll through the approved operational process;
- TLS or network failure — retry with bounded exponential backoff.

A successful command has this shape:

```json
{
  "command": {
    "command_id": "uuid",
    "type": "provider_login_start",
    "profile_id": "uuid",
    "attempt_id": "uuid",
    "expires_at": "timestamp"
  }
}
```

The command is already claimed atomically by Server A. Do not execute it if it is expired, does not match the enrolled profile, or is already present in the worker's local in-progress state. Do not assume that reserved command types (`provider_login_cancel`, `provider_health_check`, `worker_shutdown`) are enabled.

## Login executor

For `provider_login_start`:

1. Verify the profile and attempt binding and record the command ID in durable local worker state.
2. Create a dedicated protected provider home for the profile, for example `CODEX_HOME=/var/lib/worker/profiles/<profile-id>`.
3. Start the pinned provider-supported Codex login executable without shell interpolation or user-controlled executable paths.
4. Send `awaiting_authorization` with the sanitized `auth.openai.com` URL and user code.
5. Wait for authorization, enforce the Server A expiry/timeout, and terminate the complete child process tree on cancellation or timeout.
6. Send `validating` and perform provider preflight using the isolated profile credential store.
7. Send exactly one terminal event: `succeeded`, `failed`, `cancelled`, or `expired`.

Provider tokens and `auth.json` must remain on Server B. They must not appear in event bodies, command results, PostgreSQL, browser storage, container layers, or logs. Use a separate protected credential directory per profile and restrict it to the worker user.

## Event contract

Post:

```text
POST /internal/worker/events
```

The request must contain only these fields:

```json
{
  "command_id": "uuid",
  "attempt_id": "uuid",
  "state": "awaiting_authorization|validating|succeeded|failed|cancelled|expired",
  "authorization_url": "https://auth.openai.com/...",
  "user_code": "short-lived-code",
  "failure_code": "optional-code",
  "failure_message": "optional-message"
}
```

Omit fields that are not applicable or send them as empty strings. Do not send `expires_at`; Server A rejects unknown JSON fields and controls expiry itself. A successful event returns `202` with `{"status":"accepted"}`. The URL must be HTTPS on `auth.openai.com` without credentials, query, or fragment. User codes are limited to 64 alphanumeric, hyphen, or underscore characters.

## Expected state sequence

```text
provider_login_start
  -> awaiting_authorization
  -> validating
  -> succeeded
```

Failure, cancellation, worker shutdown, or timeout must produce the appropriate terminal state. Retry event delivery safely for transient network errors, but preserve the same command and attempt IDs. Do not replay a new login process for an already claimed command without checking local durable state.

## Smoke-test acceptance criteria

Run from Server B's private network:

1. Valid mTLS plus a valid worker token with no command returns `204`.
2. Missing or invalid bearer token returns `401`.
3. A test profile login creates and delivers `provider_login_start`.
4. The worker emits `awaiting_authorization`, then `validating`, then a terminal event.
5. A successful terminal event makes only the matching profile `ready` on Server A.
6. Wrong profile/attempt IDs, invalid URL, unknown JSON fields, expired commands, and revoked tokens are rejected.
7. Worker restart, provider timeout, cancellation, and certificate failure do not expose credentials or leave a profile falsely `ready`.

Use a disposable test profile, test enrollment token, and isolated provider credential directory. Never use a production provider credential for this smoke test.

## Automatic user-facing provisioning

When `WORKER_PROVISIONING_ENABLED=true`, creating a provider profile on Server A creates one encrypted, short-lived provisioning job. The browser receives no enrollment token. A Server B provisioner polls the private endpoint:

```text
GET /internal/worker/provisioning/jobs
```

This endpoint requires the separate `WORKER_PROVISIONING_TOKEN` bearer credential plus mTLS. A claimed job returns HTTP 200:

```json
{
  "job": {
    "job_id": "uuid",
    "profile_id": "uuid",
    "provider": "codex",
    "enrollment_token": "enroll_...",
    "expires_at": "timestamp"
  }
}
```

No job returns `204`. The provisioner must create the isolated worker, inject the token only into that worker, and call the existing `/internal/worker/enroll`. Successful enrollment marks the job `ready`. Provisioning failures may be reported with `POST /internal/worker/provisioning/jobs/{job_id}` and body `{"state":"failed","failure_code":"...","failure_message":"..."}`. The token is one-time and expires; it must never be sent to the browser, logged, or persisted by Server B after enrollment.

This contract requires the Server B provisioner/bootstrap agent to be deployed. The existing profile worker alone does not consume provisioning jobs yet.
