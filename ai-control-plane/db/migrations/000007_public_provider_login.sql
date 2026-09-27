BEGIN;

ALTER TABLE provider_profiles
    DROP CONSTRAINT IF EXISTS provider_profiles_status_check;

ALTER TABLE provider_profiles
    ADD CONSTRAINT provider_profiles_status_check
    CHECK (status IN ('pending', 'login_pending', 'validating', 'connecting', 'ready', 'needs_relogin', 'unhealthy', 'disabled'));

CREATE TABLE provider_login_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_profile_id uuid NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
    initiated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider text NOT NULL CHECK (provider IN ('codex', 'claude_code')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'awaiting_authorization', 'validating', 'succeeded', 'failed', 'cancelled', 'expired')),
    nonce_digest bytea NOT NULL,
    failure_code text,
    failure_message text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (id, provider_profile_id)
);

CREATE INDEX provider_login_attempts_profile_active_idx
    ON provider_login_attempts (provider_profile_id, status, expires_at);

CREATE INDEX provider_login_attempts_org_time_idx
    ON provider_login_attempts (organization_id, created_at DESC);

COMMIT;
