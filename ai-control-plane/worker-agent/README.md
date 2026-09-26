# Profile-bound worker agent scaffold

This directory contains the control-plane enrollment/heartbeat client and provider adapter interfaces. It is **not yet a production provider worker**.

## Contract

The future agent will run one isolated runtime per provider profile:

1. Receive a short-lived enrollment token from an authenticated operator.
2. Use mTLS to call the private control-plane listener.
3. Enroll with a unique `agent_id` and private `internal_url`.
4. Store the returned profile-bound worker token in a `0600` file/secret mount.
5. Send authenticated heartbeats.
6. Start exactly one provider adapter in its isolated credential home.
7. Expose its OpenAI-compatible listener only on the private worker network.

The current `internal/provider` implementations intentionally return `not implemented`. This prevents accidentally claiming that Codex or Claude multi-account isolation is complete.

Provider constraints:

- Codex requires one isolated writable `CODEX_HOME`/`auth.json` per worker; never share refresh-token files.
- Claude Code requires a verified isolated CLI home/container strategy; the current sibling server does not provide profile selection by itself.
- Provider credentials must stay in the worker runtime and must never be submitted through the browser or control-plane public API.

## Runtime configuration

`cmd/worker-agent` now supervises one provider child process per profile:

```powershell
$env:WORKER_PROVIDER = "codex"
$env:CONTROL_PLANE_URL = "https://control-plane:8081"
$env:WORKER_ENROLLMENT_TOKEN = "enroll_one_time_value"
$env:WORKER_AGENT_ID = "profile-specific-agent-001"
$env:WORKER_INTERNAL_URL = "https://worker-codex:19080"
$env:WORKER_CHILD_COMMAND = "/usr/local/bin/openai-api-server-via-codex"
$env:CODEX_HOME = "/run/provider/codex-home"
$env:CODEX_AUTH_JSON = "/run/provider/codex-home/auth.json"
$env:MTLS_CA_FILE = "/run/secrets/worker-ca.pem"
$env:MTLS_CLIENT_CERT_FILE = "/run/secrets/worker-client.pem"
$env:MTLS_CLIENT_KEY_FILE = "/run/secrets/worker-client-key.pem"
$env:WORKER_SERVER_CERT_FILE = "/run/secrets/worker-server.pem"
$env:WORKER_SERVER_KEY_FILE = "/run/secrets/worker-server-key.pem"
```

The agent binds its private HTTPS reverse proxy first, enrolls once, starts the child with the returned worker token as its local API key, waits for the child `/healthz`, then sends `online` heartbeats. On shutdown it sends `draining`, terminates the child, and closes the listener.

For Claude Code use `WORKER_PROVIDER=claude_code`, the Claude proxy executable as `WORKER_CHILD_COMMAND`, and an isolated `CLAUDE_CONFIG_DIR`. The current Claude proxy remains text-only with tools disabled. Do not share either provider home between profiles.

This runtime expects the provider proxy executable to already be present in the image. It does not download provider software, accept credentials from the browser, or create certificates. Certificate issuance/rotation and secret mounts belong to deployment infrastructure.
