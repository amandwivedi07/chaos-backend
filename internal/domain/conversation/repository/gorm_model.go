package repository

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// stringList persists a short list of labels as jsonb. Postgres arrays would
// also work, but their driver representation differs between pgx and lib/pq
// and this column is never queried by element — only read back whole.
type stringList []string

func (l stringList) Value() (driver.Value, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(l))
}

func (l *stringList) Scan(src any) error {
	if src == nil {
		*l = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("stringList: unsupported source type")
	}
	if len(raw) == 0 {
		*l = nil
		return nil
	}
	return json.Unmarshal(raw, (*[]string)(l))
}

// The GORM persistence models — the ONLY place database column mapping exists.
// None of these cross the repository boundary.

type conversationModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Emoji          string
	Title          string
	Titled         bool
	CreatedBy      uuid.UUID
	GroupID        *uuid.UUID
	Direct         bool
	Decided        stringList `gorm:"type:jsonb"`
	Open           stringList `gorm:"type:jsonb"`
	LastActivityAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

func (conversationModel) TableName() string { return "conversations" }

type memberModel struct {
	ConversationID uuid.UUID `gorm:"primaryKey"`
	UserID         uuid.UUID `gorm:"primaryKey"`
	Role           string
	JoinedAt       time.Time
	LeftAt         *time.Time
	// SeenAt is this member's read cursor: everything sent at or before it is
	// read. One timestamp beats a row per message per member.
	SeenAt *time.Time
}

func (memberModel) TableName() string { return "conversation_members" }

type messageModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ConversationID uuid.UUID
	AuthorID       *uuid.UUID
	Kind           string
	Text           string
	Action         string
	SentAt         time.Time
}

func (messageModel) TableName() string { return "messages" }

type cardModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	MessageID uuid.UUID
	Emoji     string
	Title     string
	Tagline   string
	Why       string
	Position  int
}

func (cardModel) TableName() string { return "message_cards" }

type ratingModel struct {
	CardID   uuid.UUID `gorm:"primaryKey"`
	Position int       `gorm:"primaryKey"`
	Label    string
	Stars    int
}

func (ratingModel) TableName() string { return "card_ratings" }

type decisionModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ConversationID   uuid.UUID
	MessageID        uuid.UUID
	Question         string
	ResolvedOptionID *uuid.UUID
	CreatedAt        time.Time
}

func (decisionModel) TableName() string { return "decisions" }

type optionModel struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DecisionID uuid.UUID
	Label      string
	Position   int
}

func (optionModel) TableName() string { return "decision_options" }

type voteModel struct {
	OptionID  uuid.UUID `gorm:"primaryKey"`
	UserID    uuid.UUID `gorm:"primaryKey"`
	CreatedAt time.Time
}

func (voteModel) TableName() string { return "decision_votes" }

// memberRow is the join used to build entity.Member: membership plus the
// person's own columns plus the profile line Chaos reads.
type memberRow struct {
	ConversationID uuid.UUID
	UserID         uuid.UUID
	Role           string
	Name           string
	Handle         *string
	AvatarURL      *string
	PaletteID      string
	Note           *string
	LastSeen       *time.Time
}

func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
