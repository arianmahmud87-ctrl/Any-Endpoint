BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    google_subject text NOT NULL UNIQUE,
    email text NOT NULL,
    email_verified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    owner_user_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE organization_memberships (
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE provider_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('codex', 'claude_code')),
    label text NOT NULL CHECK (length(trim(label)) BETWEEN 1 AND 120),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'connecting', 'ready', 'needs_relogin', 'unhealthy', 'disabled')),
    secret_ref text,
    allowed_models jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, provider)
);

CREATE TABLE worker_runtimes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_profile_id uuid NOT NULL UNIQUE REFERENCES provider_profiles(id) ON DELETE CASCADE,
    agent_id text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'enrolling' CHECK (status IN ('enrolling', 'online', 'offline', 'draining', 'revoked')),
    last_heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    prefix text NOT NULL UNIQUE,
    verifier bytea NOT NULL,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'retiring', 'revoked', 'expired')),
    expires_at timestamptz,
    last_used_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id),
    rotated_from uuid REFERENCES api_keys(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE TABLE api_key_grants (
    api_key_id uuid NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    provider_profile_id uuid NOT NULL,
    provider text NOT NULL CHECK (provider IN ('codex', 'claude_code')),
    model_pattern text,
    FOREIGN KEY (provider_profile_id, provider) REFERENCES provider_profiles(id, provider) ON DELETE CASCADE,
    route text NOT NULL CHECK (route IN ('models', 'responses', 'chat.completions')),
    requests_per_minute integer NOT NULL DEFAULT 60 CHECK (requests_per_minute BETWEEN 1 AND 100000),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (api_key_id, provider_profile_id, route)
);

CREATE TABLE usage_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    api_key_id uuid REFERENCES api_keys(id) ON DELETE SET NULL,
    provider_profile_id uuid REFERENCES provider_profiles(id) ON DELETE SET NULL,
    model text,
    route text NOT NULL,
    status_code integer NOT NULL CHECK (status_code BETWEEN 100 AND 599),
    latency_ms integer CHECK (latency_ms >= 0),
    input_tokens integer CHECK (input_tokens >= 0),
    output_tokens integer CHECK (output_tokens >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid REFERENCES organizations(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id uuid,
    request_id text,
    result text NOT NULL CHECK (result IN ('success', 'denied', 'failure')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX provider_profiles_org_idx ON provider_profiles (organization_id, status);
CREATE INDEX api_keys_org_state_idx ON api_keys (organization_id, state, expires_at);
CREATE INDEX api_key_grants_profile_idx ON api_key_grants (provider_profile_id);
CREATE INDEX usage_events_org_time_idx ON usage_events (organization_id, created_at DESC);
CREATE INDEX audit_events_org_time_idx ON audit_events (organization_id, created_at DESC);

COMMIT;
