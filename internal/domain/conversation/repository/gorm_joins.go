// The two fan-out reads the home list depends on: who is in each
// conversation, and the last thing said in it. Both are written to cost
// one query for the whole list rather than one per row.
package repository

import (
	"context"

	"github.com/google/uuid"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

// membersFor loads the member list for several conversations in one query.
func (r *gormRepository) membersFor(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID][]entity.Member, error) {
	var rows []memberRow
	err := r.db.WithContext(ctx).
		Table("conversation_members m").
		Select(`m.conversation_id, m.user_id, m.role,
		        u.name, u.handle, u.avatar_url, u.palette_id,
		        p.note, p.last_seen_at AS last_seen`).
		Joins("JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL").
		Joins("LEFT JOIN profiles p ON p.user_id = m.user_id").
		Where("m.conversation_id IN ? AND m.left_at IS NULL", convIDs).
		Order("m.joined_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("list members", err)
	}
	out := make(map[uuid.UUID][]entity.Member, len(convIDs))
	for _, row := range rows {
		out[row.ConversationID] = append(out[row.ConversationID], entity.Member{
			UserID:    row.UserID,
			Name:      row.Name,
			Handle:    text(row.Handle),
			AvatarURL: text(row.AvatarURL),
			PaletteID: row.PaletteID,
			Note:      text(row.Note),
			Role:      row.Role,
			LastSeen:  row.LastSeen,
		})
	}
	return out, nil
}

// lastMessages fetches one preview line per conversation with a lateral join,
// so the home list costs a single query rather than one per row.
func (r *gormRepository) lastMessages(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID]entity.Message, error) {
	var rows []messageModel
	err := r.db.WithContext(ctx).Raw(`
		SELECT last.* FROM unnest(?::uuid[]) AS c(id)
		JOIN LATERAL (
			SELECT * FROM messages
			WHERE conversation_id = c.id
			ORDER BY sent_at DESC LIMIT 1
		) AS last ON true`, uuidArray(convIDs)).Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("list previews", err)
	}
	out := make(map[uuid.UUID]entity.Message, len(rows))
	for _, m := range rows {
		out[m.ConversationID] = toMessage(m)
	}
	return out, nil
}

// messageCounts is "6 messages" on the group screen, for a whole list in one
// query.
func (r *gormRepository) messageCounts(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	var rows []struct {
		ConversationID uuid.UUID
		N              int
	}
	err := r.db.WithContext(ctx).
		Table("messages").
		Select("conversation_id, count(*) AS n").
		Where("conversation_id IN ? AND kind <> 'system'", convIDs).
		Group("conversation_id").Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("count messages", err)
	}
	out := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		out[row.ConversationID] = row.N
	}
	return out, nil
}

// groupNames resolves the badge shown on each home row, for a whole list in
// one query.
func (r *gormRepository) groupNames(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	var rows []struct {
		ConversationID uuid.UUID
		Name           string
	}
	err := r.db.WithContext(ctx).
		Table("conversations c").
		Select("c.id AS conversation_id, g.name").
		Joins("JOIN groups g ON g.id = c.group_id AND g.deleted_at IS NULL").
		Where("c.id IN ?", convIDs).Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("group names", err)
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, row := range rows {
		out[row.ConversationID] = row.Name
	}
	return out, nil
}

// uuidArray renders ids for a `= ANY(?::uuid[])` style parameter.
func uuidArray(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	out := make([]byte, 0, len(ids)*38)
	out = append(out, '{')
	for i, id := range ids {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, id.String()...)
	}
	return string(append(out, '}'))
}

// KnownPeople is everyone the caller shares a conversation with, most recent
// contact first.
func (r *gormRepository) KnownPeople(ctx context.Context, userID uuid.UUID, limit int) ([]entity.Member, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var rows []memberRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (u.id)
		       m.conversation_id, m.user_id, m.role,
		       u.name, u.handle, u.avatar_url, u.palette_id,
		       p.note, p.last_seen_at AS last_seen
		FROM conversation_members m
		JOIN users u ON u.id = m.user_id AND u.deleted_at IS NULL
		LEFT JOIN profiles p ON p.user_id = m.user_id
		WHERE m.left_at IS NULL
		  AND m.user_id <> ?
		  AND m.conversation_id IN (
		      SELECT conversation_id FROM conversation_members
		      WHERE user_id = ? AND left_at IS NULL
		  )
		ORDER BY u.id, m.joined_at DESC
		LIMIT ?`, userID, userID, limit).Scan(&rows).Error
	if err != nil {
		return nil, apperrors.Database("known people", err)
	}
	out := make([]entity.Member, len(rows))
	for i, row := range rows {
		out[i] = entity.Member{
			UserID:    row.UserID,
			Name:      row.Name,
			Handle:    text(row.Handle),
			AvatarURL: text(row.AvatarURL),
			PaletteID: row.PaletteID,
			Note:      text(row.Note),
			Role:      row.Role,
			LastSeen:  row.LastSeen,
		}
	}
	return out, nil
}
