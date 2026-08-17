-- Refresh tokens: stored HASHED (sha256) so a DB leak leaks nothing usable.
-- Rotation: on refresh, the old row is revoked and linked to its replacement;
-- presenting a revoked token is treated as theft and revokes the whole family.
CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text        NOT NULL UNIQUE,
    family_id   uuid        NOT NULL, -- rotation family for reuse detection
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    replaced_by uuid,       -- id of the token that superseded this one
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens (family_id);

-- One-time action tokens (email verification, password reset), also hashed.
CREATE TABLE action_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    text        NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_hash text        NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_action_tokens_user_purpose ON action_tokens (user_id, purpose);
