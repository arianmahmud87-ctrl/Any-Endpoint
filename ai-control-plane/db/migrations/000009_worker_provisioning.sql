BEGIN;

CREATE TABLE worker_provisioning_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_profile_id uuid NOT NULL UNIQUE REFERENCES provider_profiles(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('codex', 'claude_code')),
    enrollment_token_ciphertext bytea NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'claimed', 'ready', 'failed', 'expired')),
    expires_at timestamptz NOT NULL,
    claimed_at timestamptz,
    completed_at timestamptz,
    failure_code text,
    failure_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX worker_provisioning_jobs_queue_idx
    ON worker_provisioning_jobs (status, expires_at, created_at);

COMMIT;
