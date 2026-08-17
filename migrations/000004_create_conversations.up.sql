-- The conversation domain: a thread, what was said in it, the options Chaos
-- put forward, and the votes the group ran.

CREATE TABLE conversations (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    emoji            text        NOT NULL DEFAULT '✦',
    title            text        NOT NULL DEFAULT 'New conversation',
    -- False while the title is still a placeholder, which is what lets the
    -- header offer "Name this conversation" instead of a fake name.
    titled           boolean     NOT NULL DEFAULT false,
    created_by       uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Chaos keeping score. Rewritten wholesale each turn, never appended to,
    -- so a group that changes its mind is not haunted by the old answer.
    decided          jsonb       NOT NULL DEFAULT '[]'::jsonb,
    "open"           jsonb       NOT NULL DEFAULT '[]'::jsonb,
    last_activity_at timestamptz NOT NULL DEFAULT now(),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);

CREATE INDEX idx_conversations_activity ON conversations (last_activity_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE conversation_members (
    conversation_id uuid        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id         uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            text        NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'member')),
    joined_at       timestamptz NOT NULL DEFAULT now(),
    left_at         timestamptz,
    -- One read cursor per member beats a row per message per member.
    seen_at         timestamptz,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX idx_members_user ON conversation_members (user_id) WHERE left_at IS NULL;

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    -- NULL for Chaos turns and for the room's own lines. Neither has a user
    -- row, and giving them one would put them in the member list.
    author_id       uuid        REFERENCES users(id) ON DELETE SET NULL,
    kind            text        NOT NULL CHECK (kind IN ('user', 'chaos', 'system')),
    text            text        NOT NULL DEFAULT '',
    action          text        NOT NULL DEFAULT '',
    sent_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_conversation ON messages (conversation_id, sent_at DESC);
CREATE INDEX idx_messages_chaos ON messages (conversation_id, sent_at DESC) WHERE kind = 'chaos';

-- One comparison option under a Chaos turn.
CREATE TABLE message_cards (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id uuid NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    emoji      text NOT NULL DEFAULT '',
    title      text NOT NULL,
    tagline    text NOT NULL DEFAULT '',
    why        text NOT NULL DEFAULT '',
    position   int  NOT NULL DEFAULT 0
);

CREATE INDEX idx_cards_message ON message_cards (message_id, position);

CREATE TABLE card_ratings (
    card_id  uuid NOT NULL REFERENCES message_cards(id) ON DELETE CASCADE,
    position int  NOT NULL,
    label    text NOT NULL,
    stars    int  NOT NULL CHECK (stars BETWEEN 1 AND 5),
    PRIMARY KEY (card_id, position)
);

CREATE TABLE decisions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id    uuid        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id         uuid        NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    question           text        NOT NULL,
    -- Derived, not chosen: recomputed inside the vote transaction.
    resolved_option_id uuid,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_decisions_message ON decisions (message_id);

CREATE TABLE decision_options (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    decision_id uuid NOT NULL REFERENCES decisions(id) ON DELETE CASCADE,
    label       text NOT NULL,
    position    int  NOT NULL DEFAULT 0
);

CREATE INDEX idx_options_decision ON decision_options (decision_id, position);

-- One vote each: the primary key is the option, and the service deletes any
-- prior vote on a sibling option before inserting.
CREATE TABLE decision_votes (
    option_id  uuid        NOT NULL REFERENCES decision_options(id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (option_id, user_id)
);
