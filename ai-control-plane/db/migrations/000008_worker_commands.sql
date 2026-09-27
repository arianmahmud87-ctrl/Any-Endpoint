BEGIN;

CREATE TABLE worker_commands (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_profile_id uuid NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    provider_login_attempt_id uuid REFERENCES provider_login_attempts(id) ON DELETE CASCADE,
    command_type text NOT NULL CHECK (command_type IN ('provider_login_start', 'provider_login_cancel', 'provider_health_check', 'worker_shutdown')),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'claimed', 'completed', 'failed', 'cancelled', 'expired')),
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    claimed_at timestamptz,
    completed_at timestamptz,
    expires_at timestamptz NOT NULL,
    result_code text,
    UNIQUE (id, provider_profile_id)
);

CREATE INDEX worker_commands_profile_queue_idx
    ON worker_commands (provider_profile_id, status, created_at);

CREATE INDEX worker_commands_expiry_idx
    ON worker_commands (status, expires_at);

COMMIT;
