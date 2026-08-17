-- Push device registry (FCM tokens).
CREATE TABLE devices (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform   text        NOT NULL CHECK (platform IN ('ios', 'android')),
    push_token text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- One row per physical device: a token may migrate between accounts
    -- (same phone, different login), so the token itself is the identity.
    UNIQUE (push_token)
);

CREATE INDEX idx_devices_user ON devices (user_id);
