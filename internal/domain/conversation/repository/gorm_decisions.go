package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/conversation/entity"
)

// decisionsFor loads every decision attached to the given messages, with
// options and votes, in three queries rather than N.
func (r *gormRepository) decisionsFor(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]entity.Decision, error) {
	var models []decisionModel
	if err := r.db.WithContext(ctx).
		Where("message_id IN ?", messageIDs).Find(&models).Error; err != nil {
		return nil, apperrors.Database("list decisions", err)
	}
	if len(models) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(models))
	for i, d := range models {
		ids[i] = d.ID
	}
	options, err := r.optionsFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]entity.Decision, len(models))
	for _, d := range models {
		out[d.MessageID] = entity.Decision{
			ID:               d.ID,
			ConversationID:   d.ConversationID,
			MessageID:        d.MessageID,
			Question:         d.Question,
			ResolvedOptionID: d.ResolvedOptionID,
			Options:          options[d.ID],
		}
	}
	return out, nil
}

func (r *gormRepository) optionsFor(ctx context.Context, decisionIDs []uuid.UUID) (map[uuid.UUID][]entity.Option, error) {
	var models []optionModel
	if err := r.db.WithContext(ctx).
		Where("decision_id IN ?", decisionIDs).
		Order("position ASC").Find(&models).Error; err != nil {
		return nil, apperrors.Database("list options", err)
	}
	if len(models) == 0 {
		return nil, nil
	}
	optionIDs := make([]uuid.UUID, len(models))
	for i, o := range models {
		optionIDs[i] = o.ID
	}
	var votes []voteModel
	if err := r.db.WithContext(ctx).
		Where("option_id IN ?", optionIDs).
		Order("created_at ASC").Find(&votes).Error; err != nil {
		return nil, apperrors.Database("list votes", err)
	}
	byOption := make(map[uuid.UUID][]uuid.UUID, len(models))
	for _, v := range votes {
		byOption[v.OptionID] = append(byOption[v.OptionID], v.UserID)
	}

	out := make(map[uuid.UUID][]entity.Option, len(decisionIDs))
	for _, o := range models {
		out[o.DecisionID] = append(out[o.DecisionID], entity.Option{
			ID: o.ID, Label: o.Label, Position: o.Position, Votes: byOption[o.ID],
		})
	}
	return out, nil
}

func (r *gormRepository) GetDecision(ctx context.Context, decisionID uuid.UUID) (*entity.Decision, error) {
	var m decisionModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", decisionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NotFound("Vote not found")
		}
		return nil, apperrors.Database("get decision", err)
	}
	options, err := r.optionsFor(ctx, []uuid.UUID{m.ID})
	if err != nil {
		return nil, err
	}
	return &entity.Decision{
		ID:               m.ID,
		ConversationID:   m.ConversationID,
		MessageID:        m.MessageID,
		Question:         m.Question,
		ResolvedOptionID: m.ResolvedOptionID,
		Options:          options[m.ID],
	}, nil
}

// Vote toggles the caller's vote on one option. Everyone gets exactly one vote
// per decision, so voting for a second option moves it rather than adding one;
// voting for the option you already picked takes it back.
//
// The winner is recomputed and stored in the same transaction as the vote, so
// two people clicking at once can never leave a stale "we have a winner".
func (r *gormRepository) Vote(ctx context.Context, decisionID, optionID, userID uuid.UUID) (*entity.Decision, error) {
	var out *entity.Decision
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var option optionModel
		if err := tx.First(&option, "id = ? AND decision_id = ?", optionID, decisionID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errNoDecision
			}
			return err
		}

		// Lock the decision row so concurrent votes serialise on it.
		var decision decisionModel
		if err := tx.Raw(
			"SELECT * FROM decisions WHERE id = ? FOR UPDATE", decisionID,
		).Scan(&decision).Error; err != nil {
			return err
		}
		if decision.ID == uuid.Nil {
			return errNoDecision
		}

		var existing voteModel
		err := tx.Raw(`
			SELECT v.* FROM decision_votes v
			JOIN decision_options o ON o.id = v.option_id
			WHERE o.decision_id = ? AND v.user_id = ?`, decisionID, userID).
			Scan(&existing).Error
		if err != nil {
			return err
		}
		hadSame := existing.OptionID == optionID
		if existing.OptionID != uuid.Nil {
			if err := tx.Exec(`
				DELETE FROM decision_votes v USING decision_options o
				WHERE o.id = v.option_id AND o.decision_id = ? AND v.user_id = ?`,
				decisionID, userID).Error; err != nil {
				return err
			}
		}
		if !hadSame {
			if err := tx.Create(&voteModel{OptionID: optionID, UserID: userID}).Error; err != nil {
				return err
			}
		}

		options, err := r.optionsForTx(tx, decisionID)
		if err != nil {
			return err
		}
		fresh := entity.Decision{
			ID:             decision.ID,
			ConversationID: decision.ConversationID,
			MessageID:      decision.MessageID,
			Question:       decision.Question,
			Options:        options,
		}
		fresh.ResolvedOptionID = fresh.Resolve()
		if err := tx.Model(&decisionModel{}).Where("id = ?", decisionID).
			Update("resolved_option_id", fresh.ResolvedOptionID).Error; err != nil {
			return err
		}
		out = &fresh
		return nil
	})
	if errors.Is(err, errNoDecision) {
		return nil, apperrors.NotFound("Vote not found")
	}
	if err != nil {
		return nil, apperrors.Database("vote", err)
	}
	return out, nil
}

func (r *gormRepository) optionsForTx(tx *gorm.DB, decisionID uuid.UUID) ([]entity.Option, error) {
	var models []optionModel
	if err := tx.Where("decision_id = ?", decisionID).
		Order("position ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(models))
	for i, o := range models {
		ids[i] = o.ID
	}
	var votes []voteModel
	if len(ids) > 0 {
		if err := tx.Where("option_id IN ?", ids).
			Order("created_at ASC").Find(&votes).Error; err != nil {
			return nil, err
		}
	}
	byOption := map[uuid.UUID][]uuid.UUID{}
	for _, v := range votes {
		byOption[v.OptionID] = append(byOption[v.OptionID], v.UserID)
	}
	out := make([]entity.Option, len(models))
	for i, o := range models {
		out[i] = entity.Option{ID: o.ID, Label: o.Label, Position: o.Position, Votes: byOption[o.ID]}
	}
	return out, nil
}
