-- The profile domain: who a person is beyond their login, and the durable
-- facts Chaos has learnt about them.

CREATE TABLE profiles (
    user_id       uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bio           text        NOT NULL DEFAULT '',
    city          text        NOT NULL DEFAULT '',
    -- The one line the conversation module reads when it builds a prompt.
    -- Derived from the facts, so it is written, never edited.
    note          text        NOT NULL DEFAULT '',
    last_seen_at  timestamptz,
    -- How many of this person's own messages had been written the last time
    -- Chaos mined them. Comparing against the current count is what stops the
    -- profile screen re-reading the same history on every open.
    learned_count int         NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE facts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label        text        NOT NULL,
    value        text        NOT NULL,
    category     text        NOT NULL DEFAULT 'basics'
        CHECK (category IN ('basics', 'taste', 'constraints', 'money', 'food', 'style')),
    confidence   double precision NOT NULL DEFAULT 0.7
        CHECK (confidence >= 0 AND confidence <= 1),
    source       text        NOT NULL DEFAULT 'chaos'
        CHECK (source IN ('chaos', 'chatgpt', 'claude', 'gemini', 'you')),
    source_label text        NOT NULL DEFAULT '',
    -- The person having said "Looks right", or having typed it themselves.
    -- A confirmed fact is never overwritten by a model.
    confirmed    boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_facts_user ON facts (user_id, updated_at DESC);
-- One row per dimension: a changed mind reads as a correction, not as two
-- contradictory facts sitting next to each other.
CREATE UNIQUE INDEX idx_facts_user_label ON facts (user_id, lower(label));

CREATE TABLE ai_connections (
    user_id   uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source    text        NOT NULL CHECK (source IN ('chatgpt', 'claude', 'gemini')),
    synced_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, source)
);
