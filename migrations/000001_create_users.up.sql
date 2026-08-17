CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- An account. Email and password_hash are both optional: Apple can withhold
-- the address, and a person added to a conversation by name has neither until
-- they open the invite link.
CREATE TABLE users (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email                 text,
    firebase_uid          text,
    provider              text        NOT NULL DEFAULT 'password',
    password_hash         text,
    name                  text        NOT NULL,
    handle                text,
    role                  text        NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    -- One of the five member colours. The client maps each id to a
    -- --member-N token, so the set is closed and ordered.
    palette_id            text        NOT NULL DEFAULT 'tide'
        CHECK (palette_id IN ('tide', 'mint', 'rose', 'sun', 'iris')),
    avatar_url            text,
    email_verified_at     timestamptz,
    deletion_requested_at timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    deleted_at            timestamptz -- soft delete (GORM convention)
);

CREATE INDEX idx_users_deleted_at ON users (deleted_at);
CREATE INDEX idx_users_role ON users (role) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_name ON users (name) WHERE deleted_at IS NULL;

-- Unique where present: placeholder people have neither, and several of them
-- can coexist.
CREATE UNIQUE INDEX idx_users_email ON users (email) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX idx_users_firebase_uid ON users (firebase_uid) WHERE firebase_uid IS NOT NULL;
CREATE UNIQUE INDEX idx_users_handle ON users (handle) WHERE handle IS NOT NULL;
CREATE INDEX idx_users_email_lower ON users (lower(email));
