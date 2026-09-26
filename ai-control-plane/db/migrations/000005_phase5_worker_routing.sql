BEGIN;

ALTER TABLE worker_runtimes
    ADD COLUMN internal_url text,
    ADD COLUMN credential_ciphertext bytea;

CREATE INDEX worker_runtimes_route_idx
    ON worker_runtimes (provider_profile_id, status, last_heartbeat_at);

COMMIT;
