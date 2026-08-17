// Package service owns the profile rules: what Chaos is allowed to learn about
// a person, how a correction outranks a guess, and what gets handed to another
// assistant when someone imports their context.
package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/ai"
	"github.com/chaosapp/backend/internal/cache"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	conventity "github.com/chaosapp/backend/internal/domain/conversation/entity"
	convservice "github.com/chaosapp/backend/internal/domain/conversation/service"
	"github.com/chaosapp/backend/internal/domain/profile/dto"
	"github.com/chaosapp/backend/internal/domain/profile/entity"
	"github.com/chaosapp/backend/internal/domain/profile/repository"
	userrepo "github.com/chaosapp/backend/internal/domain/user/repository"
)

// History is the slice of the conversation module this service needs: what a
// person has said, so their profile can be mined from it. Declared here as a
// port so the profile module does not import the conversation module — the
// same shape as convservice.Facts pointing the other way.
type History interface {
	OwnMessages(ctx context.Context, userID uuid.UUID, limit int) ([]conventity.OwnMessage, error)
	CountOwnMessages(ctx context.Context, userID uuid.UUID) (int, error)
}

type Service interface {
	Get(ctx context.Context, userID uuid.UUID) (*dto.ProfileResponse, error)
	Update(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error)

	AddFact(ctx context.Context, userID uuid.UUID, req dto.AddFactRequest) (*dto.LearnResponse, error)
	UpdateFact(ctx context.Context, userID, factID uuid.UUID, req dto.UpdateFactRequest) (*dto.FactResponse, error)
	DeleteFact(ctx context.Context, userID, factID uuid.UUID) error
	Learn(ctx context.Context, userID uuid.UUID, req dto.LearnRequest) (*dto.LearnResponse, error)
	// Refresh mines what the person has said since the last time it ran. This
	// is what makes "these fill in automatically as you chat" true.
	Refresh(ctx context.Context, userID uuid.UUID) (*dto.LearnResponse, error)
	Prompt(source string) (*dto.PromptResponse, error)

	// For satisfies the conversation module's Facts port.
	For(ctx context.Context, userID uuid.UUID) ([]convservice.FactLine, error)
}

type service struct {
	repo    repository.Repository
	users   userrepo.UserRepository
	history History
	client  ai.Client
	cache   cache.Store
	log     *zap.Logger
	perHour int
}

func New(repo repository.Repository, users userrepo.UserRepository,
	history History, client ai.Client, c cache.Store,
	log *zap.Logger, perHour int) Service {
	if perHour <= 0 {
		perHour = 120
	}
	return &service{
		repo: repo, users: users, history: history,
		client: client, cache: c, log: log, perHour: perHour,
	}
}

var _ convservice.Facts = (*service)(nil)

func (s *service) Get(ctx context.Context, userID uuid.UUID) (*dto.ProfileResponse, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	facts, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountBySource(ctx, userID)
	if err != nil {
		return nil, err
	}
	connections, err := s.repo.ListConnections(ctx, userID)
	if err != nil {
		return nil, err
	}

	synced := make(map[string]time.Time, len(connections))
	for _, c := range connections {
		synced[c.Source] = c.SyncedAt
	}
	// Always answer with all three tiles, connected or not — the profile screen
	// draws the row unconditionally and an absent tile would read as a bug.
	rows := make([]dto.ConnectionResponse, 0, len(entity.ImportSources))
	for _, source := range entity.ImportSources {
		rows = append(rows, dto.ConnectionResponse{
			Source:   source,
			SyncedAt: synced[source],
			Learned:  counts[source],
		})
	}

	return &dto.ProfileResponse{
		UserID:      userID.String(),
		Name:        user.Name,
		Handle:      user.Handle,
		City:        profile.City,
		Bio:         profile.Bio,
		AvatarURL:   user.AvatarURL,
		PaletteID:   user.PaletteID,
		Note:        profile.Note,
		Facts:       toFactResponses(facts),
		Connections: rows,
	}, nil
}

func (s *service) Update(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.ProfileResponse, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	changed := false
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			user.Name, changed = name, true
		}
	}
	if req.Handle != nil {
		handle := strings.ToLower(strings.TrimSpace(*req.Handle))
		if handle != "" && handle != user.Handle {
			taken, err := s.users.HandleTaken(ctx, handle)
			if err != nil {
				return nil, err
			}
			if taken {
				return nil, apperrors.Conflict("That username is taken")
			}
			user.Handle, changed = handle, true
		}
	}
	if req.AvatarURL != nil {
		avatar := strings.TrimSpace(*req.AvatarURL)
		// Empty is how the client removes a photo. Anything else has to be a
		// real absolute URL — it ends up in an <img> for everyone in the
		// conversation, so a relative or malformed one is a broken avatar for
		// the whole group.
		if avatar != "" {
			parsed, err := url.Parse(avatar)
			if err != nil || !parsed.IsAbs() || parsed.Host == "" {
				return nil, apperrors.BadRequest("That is not a usable image address")
			}
		}
		user.AvatarURL, changed = avatar, true
	}
	if changed {
		if err := s.users.Update(ctx, user); err != nil {
			return nil, err
		}
	}

	profile, err := s.repo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	if req.City != nil {
		profile.City = strings.TrimSpace(*req.City)
	}
	if req.Bio != nil {
		profile.Bio = strings.TrimSpace(*req.Bio)
	}
	if err := s.repo.Upsert(ctx, profile); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID)
}

func (s *service) UpdateFact(ctx context.Context, userID, factID uuid.UUID, req dto.UpdateFactRequest) (*dto.FactResponse, error) {
	fact, err := s.owned(ctx, userID, factID)
	if err != nil {
		return nil, err
	}
	var value *string
	if req.Value != nil {
		trimmed := strings.TrimSpace(*req.Value)
		if trimmed == "" {
			return nil, apperrors.BadRequest("Say what it should be, or forget it")
		}
		value = &trimmed
	}
	if err := s.repo.UpdateFact(ctx, fact.ID, value, req.Confirmed); err != nil {
		return nil, err
	}
	fresh, err := s.repo.GetFact(ctx, fact.ID)
	if err != nil {
		return nil, err
	}
	s.refreshNote(ctx, userID)
	out := toFactResponse(*fresh)
	return &out, nil
}

func (s *service) DeleteFact(ctx context.Context, userID, factID uuid.UUID) error {
	fact, err := s.owned(ctx, userID, factID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteFact(ctx, fact.ID); err != nil {
		return err
	}
	s.refreshNote(ctx, userID)
	return nil
}

// owned resolves a fact and checks it belongs to the caller. Answering "not
// found" for someone else's fact is deliberate: the id space is shared, and a
// 403 would confirm the row exists.
func (s *service) owned(ctx context.Context, userID, factID uuid.UUID) (*entity.Fact, error) {
	fact, err := s.repo.GetFact(ctx, factID)
	if err != nil {
		return nil, err
	}
	if fact.UserID != userID {
		return nil, apperrors.NotFound("Input not found")
	}
	return fact, nil
}

// For flattens the facts into the shape the conversation module prompts with.
// Unconfirmed low-confidence guesses are left out: a plan built on a shaky
// inference is worse than one built on less.
func (s *service) For(ctx context.Context, userID uuid.UUID) ([]convservice.FactLine, error) {
	facts, err := s.repo.ListFacts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]convservice.FactLine, 0, len(facts))
	for _, f := range facts {
		if !f.Confirmed && f.Confidence < 0.6 {
			continue
		}
		out = append(out, convservice.FactLine{Label: f.Label, Value: f.Value})
	}
	return out, nil
}

func toFactResponse(f entity.Fact) dto.FactResponse {
	return dto.FactResponse{
		ID: f.ID.String(), Label: f.Label, Value: f.Value,
		Category: f.Category, Confidence: f.Confidence, Source: f.Source,
		SourceLabel: f.SourceLabel, Confirmed: f.Confirmed, UpdatedAt: f.UpdatedAt,
	}
}

func toFactResponses(facts []entity.Fact) []dto.FactResponse {
	out := make([]dto.FactResponse, len(facts))
	for i, f := range facts {
		out[i] = toFactResponse(f)
	}
	return out
}
