// Package service implements the authentication business logic:
// register, login, refresh rotation with reuse detection, logout,
// password flows and email verification.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	fbauth "github.com/chaosapp/backend/internal/auth/firebase"
	"github.com/chaosapp/backend/internal/auth/jwt"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/auth/dto"
	authentity "github.com/chaosapp/backend/internal/domain/auth/entity"
	"github.com/chaosapp/backend/internal/domain/auth/repository"
	"github.com/chaosapp/backend/internal/domain/device"
	userentity "github.com/chaosapp/backend/internal/domain/user/entity"
	userrepo "github.com/chaosapp/backend/internal/domain/user/repository"
	"github.com/chaosapp/backend/internal/events"
)

// AuthService is the port consumed by the auth handler.
type AuthService interface {
	Register(ctx context.Context, req dto.RegisterRequest) (*userentity.User, dto.TokenPair, error)
	Login(ctx context.Context, req dto.LoginRequest) (*userentity.User, dto.TokenPair, error)
	// SignInWithFirebase bridges Apple/Google identities to our own session.
	SignInWithFirebase(ctx context.Context, idToken string) (*userentity.User, dto.TokenPair, bool, error)
	Refresh(ctx context.Context, refreshToken string) (dto.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	ForgotPassword(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	VerifyEmail(ctx context.Context, token string) error
	ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error
	CurrentUser(ctx context.Context, userID uuid.UUID) (*userentity.User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequest) (*userentity.User, error)
	DeleteAccount(ctx context.Context, userID uuid.UUID) error
}

type authService struct {
	users     userrepo.UserRepository
	tokens    repository.TokenRepository
	devices   device.Repository
	firebase  fbauth.Verifier
	jwt       jwt.Manager
	bus       events.Bus
	log       *zap.Logger
	accessTTL time.Duration
}

var _ AuthService = (*authService)(nil)

func New(
	users userrepo.UserRepository,
	tokens repository.TokenRepository,
	devices device.Repository,
	firebaseVerifier fbauth.Verifier,
	jwtManager jwt.Manager,
	bus events.Bus,
	log *zap.Logger,
	accessTTL time.Duration,
) AuthService {
	return &authService{
		users: users, tokens: tokens, devices: devices,
		firebase: firebaseVerifier, jwt: jwtManager,
		bus: bus, log: log, accessTTL: accessTTL,
	}
}

// SignInWithFirebase verifies an Apple/Google ID token and returns our own
// session. First sign-in creates the account (isNew=true) — there is no
// separate registration step for social users.
func (s *authService) SignInWithFirebase(
	ctx context.Context, idToken string,
) (*userentity.User, dto.TokenPair, bool, error) {
	if !s.firebase.Enabled() {
		return nil, dto.TokenPair{}, false,
			&apperrors.AppError{Kind: apperrors.KindInternal,
				Message: "Sign-in is not configured on this server"}
	}
	identity, err := s.firebase.Verify(ctx, idToken)
	if err != nil {
		s.log.Warn("firebase token rejected", zap.Error(err))
		return nil, dto.TokenPair{}, false,
			apperrors.Unauthorized("That sign-in could not be verified")
	}

	user, isNew, err := s.upsertFromIdentity(ctx, identity)
	if err != nil {
		return nil, dto.TokenPair{}, false, err
	}
	pair, err := s.issuePair(ctx, user, uuid.New())
	if err != nil {
		return nil, dto.TokenPair{}, false, err
	}
	return user, pair, isNew, nil
}

// refreshFromIdentity keeps the profile in step with the identity provider on
// every sign-in: a changed Google photo should show up, and accounts created
// before handles existed get one. Best-effort — a failure here must never stop
// someone signing in.
func (s *authService) refreshFromIdentity(
	ctx context.Context, user *userentity.User, identity *fbauth.Identity,
) {
	changed := false
	// Normalised on the way in: Google offers a 96px thumbnail, which is far
	// too small for the tiles the app draws.
	if picture := utils.NormalizeAvatarURL(identity.Picture); picture != "" &&
		user.AvatarURL != picture {
		user.AvatarURL = picture
		changed = true
	}
	if identity.Name != "" && user.Name != identity.Name && user.Name == "Someone" {
		user.Name = identity.Name // Apple only ever tells us once
		changed = true
	}
	if user.Handle == "" {
		if h := uniqueHandle(ctx, s.users, handleFromEmail(user.Email)); h != "" {
			user.Handle = h
			changed = true
		}
	}
	if changed {
		if err := s.users.Update(ctx, user); err != nil {
			s.log.Warn("profile refresh failed", zap.Error(err))
		}
	}
}

// upsertFromIdentity finds the account by Firebase UID, then by email (a
// person who signed up with a password and later used Google keeps one
// account), else creates it.
func (s *authService) upsertFromIdentity(
	ctx context.Context, identity *fbauth.Identity,
) (*userentity.User, bool, error) {
	if existing, err := s.users.GetByFirebaseUID(ctx, identity.UID); err == nil {
		s.refreshFromIdentity(ctx, existing, identity)
		return existing, false, nil
	}

	if identity.Email != "" {
		if existing, err := s.users.GetByEmail(ctx, identity.Email); err == nil {
			existing.FirebaseUID = identity.UID
			existing.Provider = identity.Provider
			s.refreshFromIdentity(ctx, existing, identity)
			if err := s.users.Update(ctx, existing); err != nil {
				return nil, false, err
			}
			return existing, false, nil
		}
	}

	name := identity.Name
	if name == "" {
		name = "Someone" // Apple often withholds the name after first consent
	}
	user := &userentity.User{
		Email:       identity.Email,
		FirebaseUID: identity.UID,
		Provider:    identity.Provider,
		Name:        name,
		Handle:      uniqueHandle(ctx, s.users, handleFromEmail(identity.Email)),
		Role:        "user",
		AvatarURL:   utils.NormalizeAvatarURL(identity.Picture),
	}
	// Social identities are already verified by Apple/Google.
	if identity.Email != "" {
		now := time.Now().UTC()
		user.EmailVerifiedAt = &now
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, false, err
	}
	return user, true, nil
}

func (s *authService) Register(ctx context.Context, req dto.RegisterRequest) (*userentity.User, dto.TokenPair, error) {
	exists, err := s.users.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, dto.TokenPair{}, err
	}
	if exists {
		return nil, dto.TokenPair{}, apperrors.Conflict("An account with this email already exists")
	}

	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		return nil, dto.TokenPair{}, apperrors.Internal(err)
	}
	user := &userentity.User{
		Email: req.Email, PasswordHash: hash, Name: req.Name,
		Handle: uniqueHandle(ctx, s.users, handleFromEmail(req.Email)),
		Role:   "user",
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, dto.TokenPair{}, err
	}

	pair, err := s.issuePair(ctx, user, uuid.New())
	if err != nil {
		return nil, dto.TokenPair{}, err
	}

	verifyToken, err := s.mintActionToken(ctx, user.ID, authentity.PurposeVerifyEmail, 48*time.Hour)
	if err == nil {
		s.bus.Publish(events.Event{Name: events.UserRegistered, Payload: map[string]string{
			"email": user.Email, "name": user.Name, "token": verifyToken,
		}})
	}
	return user, pair, nil
}

func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*userentity.User, dto.TokenPair, error) {
	user, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil || !utils.CheckPassword(user.PasswordHash, req.Password) {
		// Same answer for unknown email and wrong password — no user enumeration.
		return nil, dto.TokenPair{}, apperrors.Unauthorized("Invalid email or password")
	}
	pair, err := s.issuePair(ctx, user, uuid.New())
	if err != nil {
		return nil, dto.TokenPair{}, err
	}
	return user, pair, nil
}

// Refresh rotates the refresh token. Presenting an already-revoked token is
// treated as theft: the whole rotation family is revoked.
func (s *authService) Refresh(ctx context.Context, refreshToken string) (dto.TokenPair, error) {
	claims, err := s.jwt.ParseRefresh(refreshToken)
	if err != nil {
		return dto.TokenPair{}, apperrors.Unauthorized("Invalid or expired refresh token")
	}
	row, err := s.tokens.GetRefreshByHash(ctx, utils.SHA256Hex(refreshToken))
	if err != nil {
		return dto.TokenPair{}, err
	}
	now := time.Now().UTC()
	if row.RevokedAt != nil {
		_ = s.tokens.RevokeFamily(ctx, row.FamilyID)
		s.log.Warn("refresh token reuse detected — family revoked",
			zap.String("user_id", row.UserID.String()))
		return dto.TokenPair{}, apperrors.Unauthorized("Session revoked — please sign in again")
	}
	if !row.Active(now) {
		return dto.TokenPair{}, apperrors.Unauthorized("Session expired — please sign in again")
	}

	user, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return dto.TokenPair{}, apperrors.Unauthorized("Account no longer exists")
	}
	// Claim the old token FIRST — the UPDATE ... WHERE revoked_at IS NULL is the
	// atomic gate. Losing the race means a concurrent rotation used this token,
	// which is indistinguishable from theft: burn the family.
	claimed, err := s.tokens.RevokeRefresh(ctx, row.ID, nil)
	if err != nil {
		return dto.TokenPair{}, err
	}
	if !claimed {
		_ = s.tokens.RevokeFamily(ctx, row.FamilyID)
		s.log.Warn("concurrent refresh rotation — family revoked",
			zap.String("user_id", row.UserID.String()))
		return dto.TokenPair{}, apperrors.Unauthorized("Session revoked — please sign in again")
	}
	pair, newID, err := s.issuePairWithID(ctx, user, row.FamilyID)
	if err != nil {
		return dto.TokenPair{}, err
	}
	if _, err := s.tokens.LinkReplacement(ctx, row.ID, newID); err != nil {
		s.log.Warn("could not link rotated token", zap.Error(err))
	}
	return pair, nil
}

func (s *authService) Logout(ctx context.Context, refreshToken string) error {
	row, err := s.tokens.GetRefreshByHash(ctx, utils.SHA256Hex(refreshToken))
	if err != nil {
		return nil // logging out an unknown token is fine — idempotent
	}
	_, err = s.tokens.RevokeRefresh(ctx, row.ID, nil)
	return err
}

// ---- internals ----

func (s *authService) issuePair(ctx context.Context, user *userentity.User, family uuid.UUID) (dto.TokenPair, error) {
	pair, _, err := s.issuePairWithID(ctx, user, family)
	return pair, err
}

func (s *authService) issuePairWithID(
	ctx context.Context, user *userentity.User, family uuid.UUID,
) (dto.TokenPair, uuid.UUID, error) {
	access, err := s.jwt.SignAccess(user.ID, user.Role)
	if err != nil {
		return dto.TokenPair{}, uuid.Nil, apperrors.Internal(err)
	}
	refresh, _, err := s.jwt.SignRefresh(user.ID, user.Role)
	if err != nil {
		return dto.TokenPair{}, uuid.Nil, apperrors.Internal(err)
	}
	row := &authentity.RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: utils.SHA256Hex(refresh),
		FamilyID:  family,
		ExpiresAt: time.Now().UTC().Add(s.jwt.RefreshTTL()),
	}
	if err := s.tokens.CreateRefresh(ctx, row); err != nil {
		return dto.TokenPair{}, uuid.Nil, err
	}
	return dto.TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.accessTTL.Seconds()),
	}, row.ID, nil
}

func (s *authService) mintActionToken(
	ctx context.Context, userID uuid.UUID, purpose string, ttl time.Duration,
) (string, error) {
	raw, err := utils.RandomToken(32)
	if err != nil {
		return "", apperrors.Internal(err)
	}
	_ = s.tokens.InvalidateActions(ctx, userID, purpose) // one live token per purpose
	err = s.tokens.CreateAction(ctx, &authentity.ActionToken{
		ID: uuid.New(), UserID: userID, Purpose: purpose,
		TokenHash: utils.SHA256Hex(raw), ExpiresAt: time.Now().UTC().Add(ttl),
	})
	if err != nil {
		return "", err
	}
	return raw, nil
}
