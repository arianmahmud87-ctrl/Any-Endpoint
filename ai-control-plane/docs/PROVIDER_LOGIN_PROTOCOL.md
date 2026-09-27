# Provider login protocol

This document defines the Server A/Server B contract for public multi-user provider onboarding. The current release implements the Server A login-attempt state API; the worker command channel and Codex device-code runner are the next implementation gate.

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

## Worker command contract (next gate)

Commands must be a fixed enum carried over the existing mTLS private channel:

```json
{
  "command_id": "uuid",
  "type": "provider_login_start",
  "profile_id": "uuid",
  "attempt_id": "uuid",
  "expires_at": "timestamp"
}
```

Allowed command types are `provider_login_start`, `provider_login_cancel`, `provider_health_check`, and `worker_shutdown`. The worker must reject a command if profile, attempt, organization binding, expiry, or mTLS identity does not match.

Worker events return sanitized metadata only:

```json
{
  "command_id": "uuid",
  "attempt_id": "uuid",
  "state": "awaiting_authorization",
  "authorization_url": "https://provider.example/device",
  "user_code": "short-lived-code",
  "expires_at": "timestamp"
}
```

`user_code` is short-lived sensitive data: do not persist it in PostgreSQL, logs, analytics, or browser storage. The frontend may hold it in memory until the attempt completes or expires.

## Internal worker endpoints

The mTLS-protected internal listener exposes:

```text
GET  /internal/worker/commands
POST /internal/worker/events
```

A worker polls commands with its profile-bound worker bearer token. An event request must include the command ID, attempt ID, fixed lifecycle state, and only sanitized challenge/failure metadata. Invalid worker credentials are rejected with `401`; an authenticated worker with no queued command receives `204`.