package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

func toMessage(m messageModel) entity.Message {
	return entity.Message{
		ID:             m.ID,
		ConversationID: m.ConversationID,
		AuthorID:       m.AuthorID,
		Kind:           m.Kind,
		Text:           m.Text,
		Action:         m.Action,
		SentAt:         m.SentAt,
	}
}

// ListMessages returns the tail of the thread oldest-first, with each Chaos
// turn's cards and decision already attached.
func (r *gormRepository) ListMessages(ctx context.Context, convID uuid.UUID, limit int) ([]entity.Message, error) {
	if limit <= 0 || limit > 300 {
		limit = 200
	}
	var models []messageModel
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("sent_at DESC").Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list messages", err)
	}

	// Reverse into reading order.
	out := make([]entity.Message, len(models))
	ids := make([]uuid.UUID, len(models))
	for i, m := range models {
		out[len(models)-1-i] = toMessage(m)
		ids[i] = m.ID
	}
	if len(ids) == 0 {
		return out, nil
	}

	cards, err := r.cardsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	decisions, err := r.decisionsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Cards = cards[out[i].ID]
		if d, ok := decisions[out[i].ID]; ok {
			decision := d
			out[i].Decision = &decision
		}
	}
	return out, nil
}

func (r *gormRepository) cardsFor(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID][]entity.Card, error) {
	var models []cardModel
	err := r.db.WithContext(ctx).
		Where("message_id IN ?", messageIDs).
		Order("position ASC").Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list cards", err)
	}
	if len(models) == 0 {
		return nil, nil
	}
	cardIDs := make([]uuid.UUID, len(models))
	for i, c := range models {
		cardIDs[i] = c.ID
	}
	var ratings []ratingModel
	err = r.db.WithContext(ctx).
		Where("card_id IN ?", cardIDs).
		Order("position ASC").Find(&ratings).Error
	if err != nil {
		return nil, apperrors.Database("list ratings", err)
	}
	byCard := make(map[uuid.UUID][]entity.Rating, len(models))
	for _, rt := range ratings {
		byCard[rt.CardID] = append(byCard[rt.CardID], entity.Rating{Label: rt.Label, Stars: rt.Stars})
	}

	out := make(map[uuid.UUID][]entity.Card, len(messageIDs))
	for _, c := range models {
		out[c.MessageID] = append(out[c.MessageID], entity.Card{
			ID:        c.ID,
			MessageID: c.MessageID,
			Emoji:     c.Emoji,
			Title:     c.Title,
			Tagline:   c.Tagline,
			Why:       c.Why,
			Position:  c.Position,
			Ratings:   byCard[c.ID],
		})
	}
	return out, nil
}

// InsertMessage writes a message and everything hanging off it in one
// transaction — a Chaos turn is never half-visible.
func (r *gormRepository) InsertMessage(ctx context.Context, m *entity.Message) error {
	if m.SentAt.IsZero() {
		m.SentAt = time.Now()
	}
	model := messageModel{
		ID:             uuid.New(),
		ConversationID: m.ConversationID,
		AuthorID:       m.AuthorID,
		Kind:           m.Kind,
		Text:           m.Text,
		Action:         m.Action,
		SentAt:         m.SentAt,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			return err
		}
		for i := range m.Cards {
			card := cardModel{
				ID:        uuid.New(),
				MessageID: model.ID,
				Emoji:     m.Cards[i].Emoji,
				Title:     m.Cards[i].Title,
				Tagline:   m.Cards[i].Tagline,
				Why:       m.Cards[i].Why,
				Position:  i,
			}
			if err := tx.Create(&card).Error; err != nil {
				return err
			}
			m.Cards[i].ID = card.ID
			m.Cards[i].MessageID = model.ID
			m.Cards[i].Position = i
			for j, rt := range m.Cards[i].Ratings {
				row := ratingModel{CardID: card.ID, Position: j, Label: rt.Label, Stars: rt.Stars}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
		}
		if m.Decision != nil {
			decision := decisionModel{
				ID:             uuid.New(),
				ConversationID: m.ConversationID,
				MessageID:      model.ID,
				Question:       m.Decision.Question,
				CreatedAt:      model.SentAt,
			}
			if err := tx.Create(&decision).Error; err != nil {
				return err
			}
			m.Decision.ID = decision.ID
			m.Decision.ConversationID = m.ConversationID
			m.Decision.MessageID = model.ID
			for i := range m.Decision.Options {
				option := optionModel{
					ID:         uuid.New(),
					DecisionID: decision.ID,
					Label:      m.Decision.Options[i].Label,
					Position:   i,
				}
				if err := tx.Create(&option).Error; err != nil {
					return err
				}
				m.Decision.Options[i].ID = option.ID
				m.Decision.Options[i].Position = i
			}
		}
		return tx.Model(&conversationModel{}).Where("id = ?", m.ConversationID).
			Update("last_activity_at", model.SentAt).Error
	})
	if err != nil {
		return apperrors.Database("insert message", err)
	}
	m.ID = model.ID
	return nil
}

// CountSinceChaos answers "how long has Chaos been quiet?" — the input to the
// rule that decides whether it speaks. sinceChaos is the whole history when it
// has never spoken.
func (r *gormRepository) CountSinceChaos(ctx context.Context, convID uuid.UUID) (int, int, error) {
	var row struct {
		Total    int
		LastAt   *time.Time
		SinceAll int
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT
			(SELECT count(*) FROM messages WHERE conversation_id = ?) AS total,
			(SELECT max(sent_at) FROM messages
			 WHERE conversation_id = ? AND kind = 'chaos') AS last_at`,
		convID, convID).Scan(&row).Error
	if err != nil {
		return 0, 0, apperrors.Database("count messages", err)
	}
	if row.LastAt == nil {
		return row.Total, row.Total, nil
	}
	var since int64
	err = r.db.WithContext(ctx).Model(&messageModel{}).
		Where("conversation_id = ? AND sent_at > ?", convID, *row.LastAt).
		Count(&since).Error
	if err != nil {
		return 0, 0, apperrors.Database("count since chaos", err)
	}
	return row.Total, int(since), nil
}

// GetCard returns a card and the conversation it belongs to, so the service
// can check membership before acting on "Choose this".
func (r *gormRepository) GetCard(ctx context.Context, cardID uuid.UUID) (*entity.Card, uuid.UUID, error) {
	// Flat, not an embedded cardModel: GORM's Scan does not walk an anonymous
	// embedded struct without a tag, so the card's own columns come back zeroed
	// and every "Choose this" reads as a missing card.
	var row struct {
		ID             uuid.UUID
		MessageID      uuid.UUID
		Emoji          string
		Title          string
		Tagline        string
		Why            string
		Position       int
		ConversationID uuid.UUID
	}
	err := r.db.WithContext(ctx).
		Table("message_cards c").
		Select("c.*, m.conversation_id").
		Joins("JOIN messages m ON m.id = c.message_id").
		Where("c.id = ?", cardID).
		Scan(&row).Error
	if err != nil {
		return nil, uuid.Nil, apperrors.Database("get card", err)
	}
	if row.ID == uuid.Nil {
		return nil, uuid.Nil, apperrors.NotFound("Option not found")
	}
	var ratings []ratingModel
	if err := r.db.WithContext(ctx).Where("card_id = ?", cardID).
		Order("position ASC").Find(&ratings).Error; err != nil {
		return nil, uuid.Nil, apperrors.Database("get ratings", err)
	}
	card := entity.Card{
		ID: row.ID, MessageID: row.MessageID, Emoji: row.Emoji,
		Title: row.Title, Tagline: row.Tagline, Why: row.Why, Position: row.Position,
	}
	for _, rt := range ratings {
		card.Ratings = append(card.Ratings, entity.Rating{Label: rt.Label, Stars: rt.Stars})
	}
	return &card, row.ConversationID, nil
}

var errNoDecision = errors.New("decision not found")

// OwnMessages returns everything one person has said, newest last, with the
// conversation each line came from.
//
// Only their own words: a fact mined from what someone else said would be a
// fact about the wrong person, which is the one way this feature can be
// actively harmful.
func (r *gormRepository) OwnMessages(ctx context.Context, userID uuid.UUID, limit int) ([]entity.OwnMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 80
	}
	var rows []struct {
		Title  string
		Text   string
		SentAt time.Time
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT c.title, m.text, m.sent_at
		FROM messages m
		JOIN conversations c ON c.id = m.conversation_id AND c.deleted_at IS NULL
		WHERE m.author_id = ? AND m.kind = 'user'
		ORDER BY m.sent_at DESC
		LIMIT ?`, userID, limit).Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("own messages", err)
	}
	// Reverse into the order they were said; a transcript reads forwards.
	out := make([]entity.OwnMessage, len(rows))
	for i, row := range rows {
		out[len(rows)-1-i] = entity.OwnMessage{
			ConversationTitle: row.Title, Text: row.Text, SentAt: row.SentAt,
		}
	}
	return out, nil
}

func (r *gormRepository) CountOwnMessages(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&messageModel{}).
		Where("author_id = ? AND kind = 'user'", userID).Count(&n).Error
	if err != nil {
		return 0, apperrors.Database("count own messages", err)
	}
	return int(n), nil
}
