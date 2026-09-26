BEGIN;

CREATE TABLE secret_references (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_profile_id uuid NOT NULL UNIQUE REFERENCES provider_profiles(id) ON DELETE CASCADE,
    backend text NOT NULL CHECK (backend IN ('aws_secrets_manager', 'gcore_secret_store', 'vault', 'local_dev')),
    external_ref text NOT NULL UNIQUE,
    state text NOT NULL DEFAULT 'uninitialized' CHECK (state IN ('uninitialized', 'ready', 'revoked', 'error')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE provider_profiles
    ADD COLUMN secret_reference_id uuid UNIQUE REFERENCES secret_references(id) ON DELETE SET NULL;

CREATE TABLE connection_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_profile_id uuid NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    created_by uuid NOT NULL REFERENCES users(id),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX connection_attempts_profile_idx
    ON connection_attempts (provider_profile_id, expires_at);

ALTER TABLE worker_runtimes
    ADD COLUMN credential_digest bytea UNIQUE CHECK (credential_digest IS NULL OR octet_length(credential_digest) = 32);

CREATE INDEX worker_runtimes_heartbeat_idx
    ON worker_runtimes (status, last_heartbeat_at);

COMMIT;
