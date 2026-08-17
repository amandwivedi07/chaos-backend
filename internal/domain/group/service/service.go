// Package service owns the group rules: who can see a group, what it
// remembers, and how it answers a question from everything it has said.
package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/ai"
	"github.com/chaosapp/backend/internal/cache"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/domain/group/dto"
	"github.com/chaosapp/backend/internal/domain/group/entity"
	"github.com/chaosapp/backend/internal/domain/group/repository"
	userrepo "github.com/chaosapp/backend/internal/domain/user/repository"
)

// Threads is the slice of the conversation module this service needs: the
// group's own conversations, flattened enough to answer a question from.
// Declared here as a port so the group module does not import that one.
type Threads interface {
	ForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]Thread, error)
}

// Thread is one of the group's conversations, with its recent transcript.
type Thread struct {
	ID         uuid.UUID
	Title      string
	Emoji      string
	Transcript string
}

type Service interface {
	List(ctx context.Context, userID uuid.UUID) ([]dto.GroupResponse, error)
	Get(ctx context.Context, groupID, userID uuid.UUID) (*dto.GroupResponse, error)
	Create(ctx context.Context, userID uuid.UUID, req dto.CreateGroupRequest) (*dto.GroupResponse, error)
	Update(ctx context.Context, groupID, userID uuid.UUID, req dto.UpdateGroupRequest) (*dto.GroupResponse, error)
	Delete(ctx context.Context, groupID, userID uuid.UUID) error

	AddMember(ctx context.Context, groupID, userID uuid.UUID, req dto.AddMemberRequest) (*dto.GroupResponse, error)
	RemoveMember(ctx context.Context, groupID, userID, memberID uuid.UUID) (*dto.GroupResponse, error)

	AddMemory(ctx context.Context, groupID, userID uuid.UUID, req dto.AddMemoryRequest) (*dto.GroupResponse, error)
	DeleteMemory(ctx context.Context, groupID, userID, memoryID uuid.UUID) (*dto.GroupResponse, error)

	Ask(ctx context.Context, groupID, userID uuid.UUID, req dto.AskRequest) (*dto.AnswerResponse, error)
	Collaborators(ctx context.Context, userID uuid.UUID) ([]dto.CollaboratorResponse, error)

	// Context satisfies the conversation module's GroupContext port.
	Context(ctx context.Context, groupID uuid.UUID) (name string, memory []string, err error)
}

// People is the slice of the user module this service needs, so a group can
// gain a member who has not signed up yet.
type People interface {
	Placeholder(ctx context.Context, name string) (uuid.UUID, error)
}

type service struct {
	repo    repository.Repository
	users   userrepo.UserRepository
	people  People
	threads Threads
	client  ai.Client
	cache   cache.Store
	log     *zap.Logger
	perHour int
}

func New(repo repository.Repository, users userrepo.UserRepository,
	people People, threads Threads, client ai.Client, c cache.Store,
	log *zap.Logger, perHour int) Service {
	if perHour <= 0 {
		perHour = 120
	}
	return &service{
		repo: repo, users: users, people: people, threads: threads,
		client: client, cache: c, log: log, perHour: perHour,
	}
}

func (s *service) List(ctx context.Context, userID uuid.UUID) ([]dto.GroupResponse, error) {
	groups, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.GroupResponse, len(groups))
	for i := range groups {
		out[i] = toGroupResponse(&groups[i])
	}
	return out, nil
}

func (s *service) Get(ctx context.Context, groupID, userID uuid.UUID) (*dto.GroupResponse, error) {
	g, err := s.repo.GetForUser(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	out := toGroupResponse(g)
	return &out, nil
}

func (s *service) Create(ctx context.Context, userID uuid.UUID, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperrors.BadRequest("Give the group a name")
	}
	emoji := strings.TrimSpace(req.Emoji)
	if emoji == "" {
		emoji = "✦"
	}

	members := []uuid.UUID{userID}
	for _, raw := range req.MemberIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, apperrors.BadRequest("Invalid member id")
		}
		if id != userID {
			members = append(members, id)
		}
	}

	g := &entity.Group{Name: name, Emoji: emoji, CreatedBy: userID}
	if err := s.repo.Create(ctx, g, members); err != nil {
		return nil, err
	}
	return s.Get(ctx, g.ID, userID)
}

func (s *service) Update(ctx context.Context, groupID, userID uuid.UUID, req dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	if _, err := s.repo.GetForUser(ctx, groupID, userID); err != nil {
		return nil, err
	}
	name, emoji := "", ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, apperrors.BadRequest("Give the group a name")
		}
	}
	if req.Emoji != nil {
		emoji = strings.TrimSpace(*req.Emoji)
	}
	if err := s.repo.Update(ctx, groupID, name, emoji); err != nil {
		return nil, err
	}
	return s.Get(ctx, groupID, userID)
}

func (s *service) Delete(ctx context.Context, groupID, userID uuid.UUID) error {
	g, err := s.repo.GetForUser(ctx, groupID, userID)
	if err != nil {
		return err
	}
	// Only the person who made it can dissolve it. Anyone else leaves.
	if g.CreatedBy != userID {
		return apperrors.Forbidden("Only whoever made this group can delete it")
	}
	return s.repo.Delete(ctx, groupID)
}

func (s *service) AddMember(ctx context.Context, groupID, userID uuid.UUID, req dto.AddMemberRequest) (*dto.GroupResponse, error) {
	if _, err := s.repo.GetForUser(ctx, groupID, userID); err != nil {
		return nil, err
	}

	var target uuid.UUID
	switch {
	case req.UserID != "":
		id, err := uuid.Parse(req.UserID)
		if err != nil {
			return nil, apperrors.BadRequest("Invalid member id")
		}
		if _, err := s.users.GetByID(ctx, id); err != nil {
			return nil, err
		}
		target = id
	case strings.TrimSpace(req.Name) != "":
		id, err := s.people.Placeholder(ctx, strings.TrimSpace(req.Name))
		if err != nil {
			return nil, err
		}
		target = id
	default:
		return nil, apperrors.BadRequest("Name someone to add")
	}

	if err := s.repo.AddMember(ctx, groupID, target); err != nil {
		return nil, err
	}
	return s.Get(ctx, groupID, userID)
}

func (s *service) RemoveMember(ctx context.Context, groupID, userID, memberID uuid.UUID) (*dto.GroupResponse, error) {
	if _, err := s.repo.GetForUser(ctx, groupID, userID); err != nil {
		return nil, err
	}
	if err := s.repo.RemoveMember(ctx, groupID, memberID); err != nil {
		return nil, err
	}
	// Removing yourself is leaving, and you can no longer read it back.
	if memberID == userID {
		return nil, nil
	}
	return s.Get(ctx, groupID, userID)
}

func (s *service) AddMemory(ctx context.Context, groupID, userID uuid.UUID, req dto.AddMemoryRequest) (*dto.GroupResponse, error) {
	if _, err := s.repo.GetForUser(ctx, groupID, userID); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, apperrors.BadRequest("Say what the group should remember")
	}
	author := userID
	m := &entity.Memory{GroupID: groupID, Text: text, CreatedBy: &author}
	if err := s.repo.AddMemory(ctx, m); err != nil {
		return nil, err
	}
	return s.Get(ctx, groupID, userID)
}

func (s *service) DeleteMemory(ctx context.Context, groupID, userID, memoryID uuid.UUID) (*dto.GroupResponse, error) {
	if _, err := s.repo.GetForUser(ctx, groupID, userID); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteMemory(ctx, groupID, memoryID); err != nil {
		return nil, err
	}
	return s.Get(ctx, groupID, userID)
}

func (s *service) Collaborators(ctx context.Context, userID uuid.UUID) ([]dto.CollaboratorResponse, error) {
	people, counts, err := s.repo.Collaborators(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.CollaboratorResponse, len(people))
	for i, p := range people {
		out[i] = dto.CollaboratorResponse{
			UserID:        p.UserID.String(),
			Name:          p.Name,
			Handle:        p.Handle,
			AvatarURL:     p.AvatarURL,
			PaletteID:     p.PaletteID,
			Note:          p.Note,
			Conversations: counts[i],
		}
	}
	return out, nil
}

// Context is what the conversation module reads before every Chaos turn in a
// conversation that belongs to a group. Two cheap reads on the hot path.
func (s *service) Context(ctx context.Context, groupID uuid.UUID) (string, []string, error) {
	name, err := s.repo.Name(ctx, groupID)
	if err != nil {
		return "", nil, err
	}
	memory, err := s.repo.Memory(ctx, groupID)
	if err != nil {
		return "", nil, err
	}
	lines := make([]string, 0, len(memory))
	for _, item := range memory {
		lines = append(lines, item.Text)
	}
	return name, lines, nil
}

func toGroupResponse(g *entity.Group) dto.GroupResponse {
	members := make([]dto.MemberResponse, len(g.Members))
	for i, m := range g.Members {
		members[i] = dto.MemberResponse{
			UserID:    m.UserID.String(),
			Name:      m.Name,
			Handle:    m.Handle,
			AvatarURL: m.AvatarURL,
			PaletteID: m.PaletteID,
			Note:      m.Note,
		}
	}
	memory := make([]dto.MemoryResponse, len(g.Memory))
	for i, m := range g.Memory {
		memory[i] = dto.MemoryResponse{
			ID: m.ID.String(), Text: m.Text, CreatedAt: m.CreatedAt,
		}
	}
	return dto.GroupResponse{
		ID:                g.ID.String(),
		Name:              g.Name,
		Emoji:             g.Emoji,
		Members:           members,
		Memory:            memory,
		ConversationCount: g.ConversationCount,
		CreatedAt:         g.CreatedAt,
		UpdatedAt:         g.UpdatedAt,
	}
}
