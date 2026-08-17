package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

type gormRepository struct{ db *gorm.DB }

var _ Repository = (*gormRepository)(nil)

func NewGorm(db *gorm.DB) Repository { return &gormRepository{db: db} }

// ListForUser returns every conversation the caller is still in, newest
// activity first, each with its members, unread count and last line.
func (r *gormRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entity.Conversation, error) {
	var models []conversationModel
	err := r.db.WithContext(ctx).
		Joins("JOIN conversation_members m ON m.conversation_id = conversations.id").
		Where("m.user_id = ? AND m.left_at IS NULL AND conversations.deleted_at IS NULL", userID).
		Order("conversations.last_activity_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list conversations", err)
	}
	if len(models) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	members, err := r.membersFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	previews, err := r.lastMessages(ctx, ids)
	if err != nil {
		return nil, err
	}
	unread, err := r.UnreadCounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	groups, err := r.groupNames(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]entity.Conversation, len(models))
	for i, m := range models {
		out[i] = toConversation(m)
		out[i].Members = members[m.ID]
		out[i].Unread = unread[m.ID]
		out[i].GroupName = groups[m.ID]
		if last, ok := previews[m.ID]; ok {
			msg := last
			out[i].LastMessage = &msg
		}
	}
	return out, nil
}

func (r *gormRepository) GetForUser(ctx context.Context, convID, userID uuid.UUID) (*entity.Conversation, error) {
	member, err := r.IsMember(ctx, convID, userID)
	if err != nil {
		return nil, err
	}
	if !member {
		// Not found, not forbidden: a stranger must not learn that this id is
		// a real conversation.
		return nil, apperrors.NotFound("Conversation not found")
	}
	return r.Get(ctx, convID)
}

func (r *gormRepository) Get(ctx context.Context, convID uuid.UUID) (*entity.Conversation, error) {
	var m conversationModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", convID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("Conversation not found")
		}
		return nil, apperrors.Database("get conversation", err)
	}
	members, err := r.membersFor(ctx, []uuid.UUID{convID})
	if err != nil {
		return nil, err
	}
	c := toConversation(m)
	c.Members = members[convID]
	return &c, nil
}

func toConversation(m conversationModel) entity.Conversation {
	return entity.Conversation{
		ID:        m.ID,
		Emoji:     m.Emoji,
		Title:     m.Title,
		Titled:    m.Titled,
		GroupID:   m.GroupID,
		Direct:    m.Direct,
		CreatedBy: m.CreatedBy,
		Decided:   []string(m.Decided),
		Open:      []string(m.Open),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.LastActivityAt,
	}
}

func (r *gormRepository) Create(ctx context.Context, c *entity.Conversation, memberIDs []uuid.UUID) error {
	now := time.Now()
	m := conversationModel{
		ID:             c.ID,
		Emoji:          c.Emoji,
		Title:          c.Title,
		Titled:         c.Titled,
		GroupID:        c.GroupID,
		Direct:         c.Direct,
		CreatedBy:      c.CreatedBy,
		Decided:        stringList{},
		Open:           stringList{},
		LastActivityAt: now,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		members := make([]memberModel, 0, len(memberIDs))
		for _, id := range memberIDs {
			role := entity.RoleMember
			if id == c.CreatedBy {
				role = entity.RoleOwner
			}
			members = append(members, memberModel{
				ConversationID: m.ID, UserID: id, Role: role, JoinedAt: now,
			})
		}
		if len(members) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&members).Error
	})
	if err != nil {
		return apperrors.Database("create conversation", err)
	}
	c.ID = m.ID
	c.CreatedAt = now
	c.UpdatedAt = now
	return nil
}

func (r *gormRepository) Rename(ctx context.Context, convID uuid.UUID, title, emoji string, titled bool) error {
	updates := map[string]any{"title": title, "titled": titled}
	if emoji != "" {
		updates["emoji"] = emoji
	}
	err := r.db.WithContext(ctx).Model(&conversationModel{}).
		Where("id = ?", convID).Updates(updates).Error
	if err != nil {
		return apperrors.Database("rename conversation", err)
	}
	return nil
}

func (r *gormRepository) SetGroup(ctx context.Context, convID uuid.UUID, groupID *uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&conversationModel{}).
		Where("id = ?", convID).Update("group_id", groupID).Error
	if err != nil {
		return apperrors.Database("set conversation group", err)
	}
	return nil
}

// ListForGroup returns a group's conversations, newest activity first, with
// their members attached.
func (r *gormRepository) ListForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]entity.Conversation, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var models []conversationModel
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND deleted_at IS NULL", groupID).
		Order("last_activity_at DESC").Limit(limit).
		Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list group conversations", err)
	}
	if len(models) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(models))
	for i, m := range models {
		ids[i] = m.ID
	}
	members, err := r.membersFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	counts, err := r.messageCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]entity.Conversation, len(models))
	for i, m := range models {
		out[i] = toConversation(m)
		out[i].Members = members[m.ID]
		out[i].MessageCount = counts[m.ID]
	}
	return out, nil
}

func (r *gormRepository) SetMemory(ctx context.Context, convID uuid.UUID, decided, open []string) error {
	err := r.db.WithContext(ctx).Model(&conversationModel{}).
		Where("id = ?", convID).
		Updates(map[string]any{
			"decided": stringList(decided),
			"open":    stringList(open),
		}).Error
	if err != nil {
		return apperrors.Database("set memory", err)
	}
	return nil
}

func (r *gormRepository) Delete(ctx context.Context, convID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&conversationModel{}, "id = ?", convID).Error; err != nil {
		return apperrors.Database("delete conversation", err)
	}
	return nil
}

func (r *gormRepository) Leave(ctx context.Context, convID, userID uuid.UUID) error {
	now := time.Now()
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).
		Update("left_at", now).Error
	if err != nil {
		return apperrors.Database("leave conversation", err)
	}
	return nil
}

func (r *gormRepository) IsMember(ctx context.Context, convID, userID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).
		Count(&n).Error
	if err != nil {
		return false, apperrors.Database("check membership", err)
	}
	return n > 0, nil
}

func (r *gormRepository) MemberIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("conversation_id = ? AND left_at IS NULL", convID).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, apperrors.Database("member ids", err)
	}
	return ids, nil
}

func (r *gormRepository) AddMember(ctx context.Context, convID, userID uuid.UUID) error {
	// Someone who left and is re-added gets their seat back rather than a
	// duplicate row.
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"left_at": nil}),
	}).Create(&memberModel{
		ConversationID: convID, UserID: userID,
		Role: entity.RoleMember, JoinedAt: time.Now(),
	}).Error
	if err != nil {
		return apperrors.Database("add member", err)
	}
	return nil
}

func (r *gormRepository) TouchActivity(ctx context.Context, convID uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&conversationModel{}).
		Where("id = ?", convID).Update("last_activity_at", time.Now()).Error
	if err != nil {
		return apperrors.Database("touch activity", err)
	}
	return nil
}

func (r *gormRepository) HeartbeatUser(ctx context.Context, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO profiles (user_id, last_seen_at) VALUES (?, now())
		ON CONFLICT (user_id) DO UPDATE SET last_seen_at = now()`, userID).Error
	if err != nil {
		return apperrors.Database("heartbeat", err)
	}
	return nil
}

func (r *gormRepository) MarkSeen(ctx context.Context, convID, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Update("seen_at", time.Now()).Error
	if err != nil {
		return apperrors.Database("mark seen", err)
	}
	return nil
}

// UnreadCounts counts messages after each membership's read cursor. Your own
// lines never count, and neither do system lines.
func (r *gormRepository) UnreadCounts(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]int, error) {
	var rows []struct {
		ConversationID uuid.UUID
		N              int
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT m.conversation_id, count(msg.id) AS n
		FROM conversation_members m
		JOIN messages msg ON msg.conversation_id = m.conversation_id
		     AND (m.seen_at IS NULL OR msg.sent_at > m.seen_at)
		     AND (msg.author_id IS NULL OR msg.author_id <> m.user_id)
		     AND msg.kind <> 'system'
		WHERE m.user_id = ? AND m.left_at IS NULL
		GROUP BY m.conversation_id`, userID).Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("unread counts", err)
	}
	out := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		out[row.ConversationID] = row.N
	}
	return out, nil
}
