-- Groups: the people you keep figuring things out with.
--
-- A group is not a conversation. It is a standing cast plus the context that
-- outlives any one thread — "everyone can do long weekends", "budget ceiling
-- is 2L". Conversations belong to at most one group and inherit that context.

CREATE TABLE groups (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL,
    emoji      text        NOT NULL DEFAULT '✦',
    created_by uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE TABLE group_members (
    group_id  uuid        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id   uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now(),
    left_at   timestamptz,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX idx_group_members_user ON group_members (user_id) WHERE left_at IS NULL;

-- What the group knows. Distinct from `facts`, which are about one person:
-- this is shared context, and Chaos carries it into every conversation the
-- group has.
CREATE TABLE group_memory (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   uuid        NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    text       text        NOT NULL,
    created_by uuid        REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_group_memory_group ON group_memory (group_id, created_at DESC);

-- A conversation belongs to at most one group. ON DELETE SET NULL: deleting a
-- group must not take its conversations with it — the thread is still the
-- thing people said.
ALTER TABLE conversations
    ADD COLUMN group_id uuid REFERENCES groups(id) ON DELETE SET NULL;

-- A direct conversation is you and Chaos, nobody else. It changes when Chaos
-- speaks: there is no group to wait for, so it answers every message.
ALTER TABLE conversations
    ADD COLUMN direct boolean NOT NULL DEFAULT false;

CREATE INDEX idx_conversations_group ON conversations (group_id)
    WHERE group_id IS NOT NULL AND deleted_at IS NULL;
