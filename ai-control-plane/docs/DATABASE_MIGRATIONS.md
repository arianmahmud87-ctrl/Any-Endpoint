# Database migrations

PostgreSQL migrations are applied in lexical order from `db/migrations/`.

The initial migration creates ownership, profile, worker, key-grant, usage, and audit foundations. It intentionally stores only verification material for future API keys; raw API key secrets and provider credentials are not part of the schema.

For local Compose, files are mounted into PostgreSQL's initialization directory. For an existing database, use a reviewed migration runner in CI/deployment rather than copying SQL manually. A future phase will add a migration table/runner and real database connectivity checks.


## Phase 2

`000002_phase2_auth.sql` adds the personal-workspace invariant and durable server-side sessions. Apply migrations in lexical order. The session table stores only an HMAC digest of the opaque browser session identifier; it does not store Google tokens or raw cookie values. OAuth state/nonce/PKCE data is short-lived in Redis and is consumed once.


## Phase 3

`000003_phase3_profiles_workers.sql` adds opaque secret-reference metadata, one-time connection attempts, and a digest-only worker credential field. It deliberately does not add provider access-token, refresh-token, auth-file, or password columns. Apply it only after `000001` and `000002`.


## Phase 4

`000004_phase4_api_keys.sql` adds the key type discriminator and lookup index to the existing API-key/grant tables. The initial schema already contains the verifier and grant tables; migration 000004 does not add raw secret material.


## Phase 5

`000005_phase5_worker_routing.sql` adds an internal worker URL and AES-GCM ciphertext field to `worker_runtimes`. The worker transport token is encrypted at rest with the configured key pepper and is never returned by profile APIs. The heartbeat digest remains separate for token verification.


## Phase 6

No new schema is required for the Phase 6 operations foundation. It consumes the existing tenant-scoped `usage_events`, `audit_events`, `provider_profiles`, `worker_runtimes`, and `api_keys` tables. A production deployment still needs a reviewed retention/partition job for usage and audit growth; this phase only caps API result size.

## Phase 7 — provider login attempts

`000007_public_provider_login.sql` adds the organization/profile-scoped provider login attempt state used by the direct device-code onboarding flow. It stores only a keyed nonce digest and sanitized lifecycle/failure metadata; provider tokens, device codes, and `auth.json` are not stored in PostgreSQL. Apply it after `000006` with the reviewed production migration procedure and take a backup first.

## Phase 8 — worker commands

`000008_worker_commands.sql` adds the profile-bound, expiring worker command queue used by provider login and health commands. Commands contain only fixed command types and server-generated IDs; provider credentials and device-code challenge values are not stored in PostgreSQL.

## 000009 — automatic worker provisioning

`000009_worker_provisioning.sql` creates the encrypted worker provisioning outbox. It stores only the encrypted one-time enrollment token, profile/provider binding, lease status, expiry, and sanitized failure metadata. Apply this additive migration before enabling `WORKER_PROVISIONING_ENABLED`; existing databases are not upgraded automatically by Compose startup.
