# Provider login protocol

This document defines the Server A/Server B contract for public multi-user provider onboarding. Server A implements the login-attempt state API and the mTLS-protected worker command/event channel. Server B must provide the worker poller and provider login runner described below.

## Security boundary

- The browser may receive only a short-lived, sanitized provider challenge.
- Provider access tokens, refresh tokens, `auth.json`, worker bearer tokens, mTLS private keys, and SSH keys never enter the browser.
- Every login attempt is bound to one organization, one profile, one initiating user, one session, and one expiry.
- The worker is reached only over the private WireGuard/mTLS channel.
- A user-controlled value must never become a shell command, executable path, environment variable name, filesystem path, or SQL fragment.

## Server A management API

```text
POST /api/profiles/{profile_id}/login/start
GET  /api/profiles/{profile_id}/login/status?attempt_id={attempt_id}
POST /api/profiles/{profile_id}/login/cancel?attempt_id={attempt_id}
POST /api/profiles/{profile_id}/login/retry?attempt_id={attempt_id}
```

Start/cancel/retry require an authenticated owner/admin session and CSRF token. Status requires an authenticated organization member session. The attempt is short-lived and only one active attempt may exist for a profile.

Initial start response:

```json
{
  "login": {
    "id": "attempt-uuid",
    "profile_id": "profile-uuid",
    "provider": "codex",
    "status": "pending",
    "expires_at": "timestamp"
  },
  "next_action": "worker_login_dispatch_pending"
}
```

The current response intentionally contains no provider URL, device code, token, or credential. Those fields may be added only through an encrypted/short-lived challenge channel after the worker login runner is implemented.

## State machine

```text
pending
  → awaiting_authorization
  → validating
  → succeeded

pending/awaiting_authorization/validating
  → cancelled
  → failed
  → expired

succeeded → ready profile
failed/expired → needs_relogin or pending
```

A profile is not routable until its provider authentication has been validated and the profile status is `ready`. Worker transport `online` alone is not sufficient.

## Worker command contract

Commands are delivered through the existing mTLS private channel. A successful poll returns HTTP 200 with this envelope:

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

`attempt_id` is omitted when a command has no login attempt. When the authenticated worker has no non-expired queued command, the response is HTTP 204 with an empty body. Invalid or revoked bearer credentials return HTTP 401.

The current login path creates `provider_login_start`. The command table also reserves `provider_login_cancel`, `provider_health_check`, and `worker_shutdown`; Server B must not execute those types until their behavior is explicitly enabled by Server A.

Worker events return sanitized metadata only. The accepted request fields are exactly:

```json
{
  "command_id": "uuid",
  "attempt_id": "uuid",
  "state": "awaiting_authorization",
  "authorization_url": "https://auth.openai.com/...",
  "user_code": "short-lived-code",
  "failure_code": "optional-code",
  "failure_message": "optional-message"
}
```

Unknown JSON fields are rejected. `expires_at` is not an accepted event field; the attempt expiry is controlled by Server A. A valid event returns HTTP 202:

```json
{"status":"accepted"}
```

The event URL, when supplied, must be HTTPS on `auth.openai.com`, without credentials, query, or fragment. `user_code` is limited to 64 alphanumeric, hyphen, or underscore characters. `user_code` is short-lived sensitive data: do not persist it in PostgreSQL, logs, analytics, or browser storage. The frontend may hold it in memory until the attempt completes or expires.

## Internal worker endpoints

The mTLS-protected internal listener exposes:

```text
POST /internal/worker/enroll
GET  /internal/worker/commands
POST /internal/worker/events
POST /internal/worker/heartbeat
```

`/internal/worker/enroll` consumes a one-time enrollment token and returns a one-time profile-bound `worker_token`. Subsequent command, event, and heartbeat requests require both the mTLS client certificate and:

```http
Authorization: Bearer <worker_token>
```

A worker polls commands with its profile-bound token. An event request must include the command ID, attempt ID, fixed lifecycle state, and only sanitized challenge/failure metadata. Invalid worker credentials are rejected with `401`; an authenticated worker with no queued command receives `204`. The worker must fail closed on token or certificate failure and must not substitute a public endpoint for the private listener.

## Automatic worker provisioning

When enabled by `WORKER_PROVISIONING_ENABLED=true`, profile creation creates an encrypted, short-lived provisioning job. The browser receives no enrollment token. A Server B bootstrap provisioner claims jobs over the private mTLS listener using the separate `WORKER_PROVISIONING_TOKEN` bearer credential:

```text
GET /internal/worker/provisioning/jobs
POST /internal/worker/provisioning/jobs/{job_id}
```

A claimed job contains only the profile/provider binding, a one-time enrollment token, and expiry. The provisioner injects the token into an isolated worker and calls `POST /internal/worker/enroll`; successful enrollment marks the job ready. The provisioner must not log, persist, or return the token to a browser. A failed job may be reported with state `failed` and sanitized failure metadata. This is a bootstrap contract for Server B and is separate from the profile-bound command/event token.
