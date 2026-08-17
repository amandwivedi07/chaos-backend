package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/group/entity"
)

type groupModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string
	Emoji     string
	CreatedBy uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (groupModel) TableName() string { return "groups" }

type memberModel struct {
	GroupID  uuid.UUID `gorm:"primaryKey"`
	UserID   uuid.UUID `gorm:"primaryKey"`
	JoinedAt time.Time
	LeftAt   *time.Time
}

func (memberModel) TableName() string { return "group_members" }

type memoryModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	GroupID   uuid.UUID
	Text      string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

func (memoryModel) TableName() string { return "group_memory" }

// memberRow is the join that builds entity.Member: membership plus the
// person's columns plus the one profile line Chaos reads.
type memberRow struct {
	GroupID   uuid.UUID
	UserID    uuid.UUID
	Name      string
	Handle    *string
	AvatarURL *string
	PaletteID string
	Note      *string
}

func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type gormRepository struct{ db *gorm.DB }

var _ Repository = (*gormRepository)(nil)

func NewGorm(db *gorm.DB) Repository { return &gormRepository{db: db} }

func toGroup(m groupModel) entity.Group {
	return entity.Group{
		ID: m.ID, Name: m.Name, Emoji: m.Emoji, CreatedBy: m.CreatedBy,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func (r *gormRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]entity.Group, error) {
	var models []groupModel
	err := r.db.WithContext(ctx).
		Joins("JOIN group_members m ON m.group_id = groups.id").
		Where("m.user_id = ? AND m.left_at IS NULL AND groups.deleted_at IS NULL", userID).
		Order("groups.updated_at DESC").
		Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list groups", err)
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
	counts, err := r.conversationCounts(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]entity.Group, len(models))
	for i, m := range models {
		out[i] = toGroup(m)
		out[i].Members = members[m.ID]
		out[i].ConversationCount = counts[m.ID]
	}
	return out, nil
}

func (r *gormRepository) GetForUser(ctx context.Context, groupID, userID uuid.UUID) (*entity.Group, error) {
	member, err := r.IsMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if !member {
		// Not found rather than forbidden: a stranger must not learn the id is
		// real.
		return nil, apperrors.NotFound("Group not found")
	}

	var m groupModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("Group not found")
		}
		return nil, apperrors.Database("get group", err)
	}
	members, err := r.membersFor(ctx, []uuid.UUID{groupID})
	if err != nil {
		return nil, err
	}
	memory, err := r.Memory(ctx, groupID)
	if err != nil {
		return nil, err
	}
	counts, err := r.conversationCounts(ctx, []uuid.UUID{groupID})
	if err != nil {
		return nil, err
	}

	g := toGroup(m)
	g.Members = members[groupID]
	g.Memory = memory
	g.ConversationCount = counts[groupID]
	return &g, nil
}

func (r *gormRepository) membersFor(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID][]entity.Member, error) {
	var rows []memberRow
	err := r.db.WithContext(ctx).
		Table("group_members m").
		Select(`m.group_id, m.user_id, u.name, u.handle, u.avatar_url,
		        u.palette_id, p.note`).
		Joins("JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL").
		Joins("LEFT JOIN profiles p ON p.user_id = m.user_id").
		Where("m.group_id IN ? AND m.left_at IS NULL", groupIDs).
		Order("m.joined_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("list group members", err)
	}
	out := make(map[uuid.UUID][]entity.Member, len(groupIDs))
	for _, row := range rows {
		out[row.GroupID] = append(out[row.GroupID], entity.Member{
			UserID:    row.UserID,
			Name:      row.Name,
			Handle:    text(row.Handle),
			AvatarURL: text(row.AvatarURL),
			PaletteID: row.PaletteID,
			Note:      text(row.Note),
		})
	}
	return out, nil
}

func (r *gormRepository) conversationCounts(ctx context.Context, groupIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	var rows []struct {
		GroupID uuid.UUID
		N       int
	}
	err := r.db.WithContext(ctx).
		Table("conversations").
		Select("group_id, count(*) AS n").
		Where("group_id IN ? AND deleted_at IS NULL", groupIDs).
		Group("group_id").Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("count group conversations", err)
	}
	out := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		out[row.GroupID] = row.N
	}
	return out, nil
}

func (r *gormRepository) Create(ctx context.Context, g *entity.Group, memberIDs []uuid.UUID) error {
	now := time.Now()
	m := groupModel{
		ID: uuid.New(), Name: g.Name, Emoji: g.Emoji,
		CreatedBy: g.CreatedBy, CreatedAt: now, UpdatedAt: now,
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		members := make([]memberModel, 0, len(memberIDs))
		for _, id := range memberIDs {
			members = append(members, memberModel{GroupID: m.ID, UserID: id, JoinedAt: now})
		}
		if len(members) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&members).Error
	})
	if err != nil {
		return apperrors.Database("create group", err)
	}
	g.ID = m.ID
	g.CreatedAt = now
	g.UpdatedAt = now
	return nil
}

func (r *gormRepository) Update(ctx context.Context, groupID uuid.UUID, name, emoji string) error {
	updates := map[string]any{"updated_at": time.Now()}
	if name != "" {
		updates["name"] = name
	}
	if emoji != "" {
		updates["emoji"] = emoji
	}
	err := r.db.WithContext(ctx).Model(&groupModel{}).
		Where("id = ?", groupID).Updates(updates).Error
	if err != nil {
		return apperrors.Database("update group", err)
	}
	return nil
}

func (r *gormRepository) Delete(ctx context.Context, groupID uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&groupModel{}, "id = ?", groupID).Error; err != nil {
		return apperrors.Database("delete group", err)
	}
	return nil
}

func (r *gormRepository) IsMember(ctx context.Context, groupID, userID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("group_id = ? AND user_id = ? AND left_at IS NULL", groupID, userID).
		Count(&n).Error
	if err != nil {
		return false, apperrors.Database("check group membership", err)
	}
	return n > 0, nil
}

func (r *gormRepository) AddMember(ctx context.Context, groupID, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"left_at": nil}),
	}).Create(&memberModel{GroupID: groupID, UserID: userID, JoinedAt: time.Now()}).Error
	if err != nil {
		return apperrors.Database("add group member", err)
	}
	return nil
}

func (r *gormRepository) RemoveMember(ctx context.Context, groupID, userID uuid.UUID) error {
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("group_id = ? AND user_id = ? AND left_at IS NULL", groupID, userID).
		Update("left_at", time.Now()).Error
	if err != nil {
		return apperrors.Database("remove group member", err)
	}
	return nil
}

func (r *gormRepository) MemberIDs(ctx context.Context, groupID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).Model(&memberModel{}).
		Where("group_id = ? AND left_at IS NULL", groupID).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, apperrors.Database("group member ids", err)
	}
	return ids, nil
}

func (r *gormRepository) AddMemory(ctx context.Context, m *entity.Memory) error {
	model := memoryModel{
		ID: uuid.New(), GroupID: m.GroupID, Text: m.Text,
		CreatedBy: m.CreatedBy, CreatedAt: time.Now(),
	}
	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		return apperrors.Database("add group memory", err)
	}
	m.ID = model.ID
	m.CreatedAt = model.CreatedAt
	return nil
}

func (r *gormRepository) DeleteMemory(ctx context.Context, groupID, memoryID uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Delete(&memoryModel{}, "id = ? AND group_id = ?", memoryID, groupID).Error
	if err != nil {
		return apperrors.Database("delete group memory", err)
	}
	return nil
}

func (r *gormRepository) Memory(ctx context.Context, groupID uuid.UUID) ([]entity.Memory, error) {
	var models []memoryModel
	err := r.db.WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("created_at DESC").Find(&models).Error
	if err != nil {
		return nil, apperrors.Database("list group memory", err)
	}
	out := make([]entity.Memory, len(models))
	for i, m := range models {
		out[i] = entity.Memory{
			ID: m.ID, GroupID: m.GroupID, Text: m.Text,
			CreatedBy: m.CreatedBy, CreatedAt: m.CreatedAt,
		}
	}
	return out, nil
}

func (r *gormRepository) Name(ctx context.Context, groupID uuid.UUID) (string, error) {
	var name string
	err := r.db.WithContext(ctx).Model(&groupModel{}).
		Where("id = ?", groupID).Limit(1).Pluck("name", &name).Error
	if err != nil {
		return "", apperrors.Database("group name", err)
	}
	return name, nil
}

// Collaborators ranks people by how many conversations you are both in.
//
// It reads conversation membership, not group membership: the people you
// actually talk to are a better answer than the people someone once added to
// a group.
func (r *gormRepository) Collaborators(ctx context.Context, userID uuid.UUID) ([]entity.Member, []int, error) {
	var rows []struct {
		UserID    uuid.UUID
		Name      string
		Handle    *string
		AvatarURL *string
		PaletteID string
		Note      *string
		N         int
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT u.id AS user_id, u.name, u.handle, u.avatar_url, u.palette_id,
		       p.note, count(*) AS n
		FROM conversation_members them
		JOIN conversation_members mine
		  ON mine.conversation_id = them.conversation_id
		 AND mine.user_id = ? AND mine.left_at IS NULL
		JOIN users u ON u.id = them.user_id AND u.deleted_at IS NULL
		LEFT JOIN profiles p ON p.user_id = them.user_id
		WHERE them.left_at IS NULL AND them.user_id <> ?
		GROUP BY u.id, u.name, u.handle, u.avatar_url, u.palette_id, p.note
		ORDER BY n DESC, u.name ASC
		LIMIT 50`, userID, userID).Scan(&rows).Error
	if err != nil {
		return nil, nil, apperrors.Database("collaborators", err)
	}
	people := make([]entity.Member, len(rows))
	counts := make([]int, len(rows))
	for i, row := range rows {
		people[i] = entity.Member{
			UserID:    row.UserID,
			Name:      row.Name,
			Handle:    text(row.Handle),
			AvatarURL: text(row.AvatarURL),
			PaletteID: row.PaletteID,
			Note:      text(row.Note),
		}
		counts[i] = row.N
	}
	return people, counts, nil
}
