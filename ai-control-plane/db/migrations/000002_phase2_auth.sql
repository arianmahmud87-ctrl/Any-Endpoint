BEGIN;

ALTER TABLE organizations
    ADD COLUMN personal boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX organizations_personal_owner_uidx
    ON organizations (owner_user_id)
    WHERE personal = true;

CREATE INDEX organization_memberships_user_idx
    ON organization_memberships (user_id, organization_id);

CREATE TABLE sessions (
    session_digest bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_agent text NOT NULL DEFAULT '',
    ip_hash text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (octet_length(session_digest) = 32)
);

CREATE INDEX sessions_user_active_idx
    ON sessions (user_id, revoked_at, expires_at);
CREATE INDEX sessions_expiry_idx
    ON sessions (expires_at);

COMMIT;
