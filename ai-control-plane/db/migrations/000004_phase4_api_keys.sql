BEGIN;

ALTER TABLE api_keys
    ADD COLUMN key_type text NOT NULL DEFAULT 'universal'
    CHECK (key_type IN ('codex', 'claude', 'universal'));

CREATE INDEX api_keys_prefix_state_idx
    ON api_keys (prefix, state);

COMMIT;
