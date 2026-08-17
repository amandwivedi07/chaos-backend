// Package service holds ALL user business logic behind the UserService port.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/cache"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/request"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/user/dto"
	"github.com/chaosapp/backend/internal/domain/user/entity"
	"github.com/chaosapp/backend/internal/domain/user/repository"
)

// UserService is the business port consumed by handlers (and the auth module).
type UserService interface {
	Create(ctx context.Context, req dto.CreateUserRequest) (*entity.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error)
	Update(ctx context.Context, id uuid.UUID, req dto.UpdateUserRequest) (*entity.User, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, p request.Pagination, f repository.ListFilter) ([]entity.User, int64, error)
}

type userService struct {
	repo  repository.UserRepository
	cache cache.Store
	log   *zap.Logger
}

var _ UserService = (*userService)(nil)

func New(repo repository.UserRepository, store cache.Store, log *zap.Logger) UserService {
	return &userService{repo: repo, cache: store, log: log}
}

const cacheTTL = 5 * time.Minute

func cacheKey(id uuid.UUID) string { return "user:" + id.String() }

func (s *userService) Create(ctx context.Context, req dto.CreateUserRequest) (*entity.User, error) {
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	role := req.Role
	if role == "" {
		role = "user"
	}
	user := &entity.User{
		Email:        req.Email,
		PasswordHash: hash,
		Name:         req.Name,
		Role:         role,
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// GetByID uses cache-aside: Redis hit first, DB on miss, then populate.
func (s *userService) GetByID(ctx context.Context, id uuid.UUID) (*entity.User, error) {
	if raw, ok, _ := s.cache.Get(ctx, cacheKey(id)); ok {
		var u entity.User
		if err := json.Unmarshal([]byte(raw), &u); err == nil {
			return &u, nil
		}
	}
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if raw, err := json.Marshal(user); err == nil {
		_ = s.cache.Set(ctx, cacheKey(id), string(raw), cacheTTL)
	}
	return user, nil
}

func (s *userService) Update(ctx context.Context, id uuid.UUID, req dto.UpdateUserRequest) (*entity.User, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.AvatarURL != nil {
		user.AvatarURL = *req.AvatarURL
	}
	if req.Role != nil {
		user.Role = *req.Role
	}
	if err := s.repo.Update(ctx, user); err != nil {
		return nil, err
	}
	_ = s.cache.Delete(ctx, cacheKey(id)) // invalidate on write
	return user, nil
}

func (s *userService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return err
	}
	_ = s.cache.Delete(ctx, cacheKey(id))
	return nil
}

func (s *userService) List(
	ctx context.Context, p request.Pagination, f repository.ListFilter,
) ([]entity.User, int64, error) {
	return s.repo.List(ctx, p, f)
}
