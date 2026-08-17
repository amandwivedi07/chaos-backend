package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/utils"
	"github.com/chaosapp/backend/internal/domain/auth/dto"
	authentity "github.com/chaosapp/backend/internal/domain/auth/entity"
	userentity "github.com/chaosapp/backend/internal/domain/user/entity"
	"github.com/chaosapp/backend/internal/events"
)

// ForgotPassword always answers success — never reveals whether the email
// exists. If it does, a one-time reset token is minted and emailed (async).
func (s *authService) ForgotPassword(ctx context.Context, email string) error {
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil // silent for unknown emails
	}
	token, err := s.mintActionToken(ctx, user.ID, authentity.PurposeResetPassword, 30*time.Minute)
	if err != nil {
		return err
	}
	s.bus.Publish(events.Event{Name: events.PasswordResetRequested, Payload: map[string]string{
		"email": user.Email, "name": user.Name, "token": token,
	}})
	return nil
}

// ResetPassword consumes a one-time reset token, sets the new password and
// signs the user out everywhere.
func (s *authService) ResetPassword(ctx context.Context, token, newPassword string) error {
	action, err := s.tokens.GetActionByHash(ctx, authentity.PurposeResetPassword, utils.SHA256Hex(token))
	if err != nil {
		return err
	}
	if !action.Usable(time.Now().UTC()) {
		return apperrors.BadRequest("Invalid or expired token")
	}
	user, err := s.users.GetByID(ctx, action.UserID)
	if err != nil {
		return err
	}
	hash, err := utils.HashPassword(newPassword)
	if err != nil {
		return apperrors.Internal(err)
	}
	user.PasswordHash = hash
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}
	_ = s.tokens.MarkActionUsed(ctx, action.ID)
	return s.tokens.RevokeAllForUser(ctx, user.ID)
}

// VerifyEmail consumes a verification token and stamps email_verified_at.
func (s *authService) VerifyEmail(ctx context.Context, token string) error {
	action, err := s.tokens.GetActionByHash(ctx, authentity.PurposeVerifyEmail, utils.SHA256Hex(token))
	if err != nil {
		return err
	}
	if !action.Usable(time.Now().UTC()) {
		return apperrors.BadRequest("Invalid or expired token")
	}
	user, err := s.users.GetByID(ctx, action.UserID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	user.EmailVerifiedAt = &now
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}
	return s.tokens.MarkActionUsed(ctx, action.ID)
}

// ChangePassword verifies the current password, sets the new one and revokes
// every other session.
func (s *authService) ChangePassword(ctx context.Context, userID uuid.UUID, current, next string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !utils.CheckPassword(user.PasswordHash, current) {
		return apperrors.Unauthorized("Current password is incorrect")
	}
	hash, err := utils.HashPassword(next)
	if err != nil {
		return apperrors.Internal(err)
	}
	user.PasswordHash = hash
	if err := s.users.Update(ctx, user); err != nil {
		return err
	}
	return s.tokens.RevokeAllForUser(ctx, userID)
}

// DeleteAccount scrubs the account, ends every session and stops all push.
// Required by App Store review; irreversible from the client's perspective.
func (s *authService) DeleteAccount(ctx context.Context, userID uuid.UUID) error {
	if err := s.tokens.RevokeAllForUser(ctx, userID); err != nil {
		return err
	}
	// Remove the Apple/Google identity too, so signing in again starts fresh.
	if user, err := s.users.GetByID(ctx, userID); err == nil && user.FirebaseUID != "" {
		if err := s.firebase.DeleteUser(ctx, user.FirebaseUID); err != nil {
			s.log.Warn("could not delete firebase identity", zap.Error(err))
		}
	}
	if s.devices != nil {
		if err := s.devices.DeleteAllForUser(ctx, userID); err != nil {
			s.log.Warn("could not clear push devices on delete", zap.Error(err))
		}
	}
	return s.users.AnonymizeAndDelete(ctx, userID)
}

func (s *authService) CurrentUser(ctx context.Context, userID uuid.UUID) (*userentity.User, error) {
	return s.users.GetByID(ctx, userID)
}

// UpdateProfile lets the authenticated user change their own name/avatar.
func (s *authService) UpdateProfile(
	ctx context.Context, userID uuid.UUID, req dto.UpdateMeRequest,
) (*userentity.User, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		user.Name = *req.Name
	}
	if req.AvatarURL != nil {
		user.AvatarURL = *req.AvatarURL
	}
	if req.PaletteID != nil {
		user.PaletteID = *req.PaletteID
	}
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}
