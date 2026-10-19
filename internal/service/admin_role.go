package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/google/uuid"
)

var (
	// ErrAdminTargetNotFound is returned when the target account does not exist.
	ErrAdminTargetNotFound = errors.New("user not found")

	// ErrAdminTargetUnverified is returned when granting admin rights to an
	// account whose email address is not verified.
	ErrAdminTargetUnverified = errors.New("account email address is not verified")

	// ErrAdminTargetDisabled is returned when granting admin rights to a
	// disabled account.
	ErrAdminTargetDisabled = errors.New("account is disabled")

	// ErrLastAdmin is returned when revoking, disabling or deleting the last
	// verified, enabled admin.
	ErrLastAdmin = errors.New("cannot remove the last admin")

	// ErrBootstrapAdmin is returned when revoking, disabling, deleting or
	// resetting 2FA on the account that holds the ADMIN_EMAIL address.
	ErrBootstrapAdmin = errors.New("account holds the ADMIN_EMAIL address")
)

// IsAdmin reports whether the user is an admin: a verified account that
// carries the admin flag or holds bootstrapEmail (case-insensitive).
func IsAdmin(user *models.User, bootstrapEmail string) bool {
	if user == nil || !user.EmailVerified {
		return false
	}
	return user.IsAdmin || IsBootstrapAdmin(user, bootstrapEmail)
}

// IsBootstrapAdmin reports whether the user is a verified holder of
// bootstrapEmail.
func IsBootstrapAdmin(user *models.User, bootstrapEmail string) bool {
	return user != nil && user.EmailVerified && bootstrapEmail != "" &&
		strings.EqualFold(user.Email, bootstrapEmail)
}

// AdminRoleService owns admin rights: the per-account flag and the
// ADMIN_EMAIL bootstrap address, whose verified holder is always an admin.
type AdminRoleService struct {
	admins         repository.AdminRepository
	bootstrapEmail string
}

func NewAdminRoleService(admins repository.AdminRepository, bootstrapEmail string) *AdminRoleService {
	return &AdminRoleService{admins: admins, bootstrapEmail: bootstrapEmail}
}

// BootstrapEmail returns the configured ADMIN_EMAIL.
func (s *AdminRoleService) BootstrapEmail() string { return s.bootstrapEmail }

// IsAdmin reports whether user is an admin under this service's ADMIN_EMAIL.
func (s *AdminRoleService) IsAdmin(user *models.User) bool { return IsAdmin(user, s.bootstrapEmail) }

// IsBootstrapAdmin reports whether user is the verified ADMIN_EMAIL holder.
func (s *AdminRoleService) IsBootstrapAdmin(user *models.User) bool {
	return IsBootstrapAdmin(user, s.bootstrapEmail)
}

// CountAdmins counts verified, enabled admins.
func (s *AdminRoleService) CountAdmins(ctx context.Context) (int, error) {
	return s.admins.CountAdmins(ctx, s.bootstrapEmail)
}

// ListUsers returns one page of the admin user listing; filter.IsAdmin
// matches IsAdmin.
func (s *AdminRoleService) ListUsers(ctx context.Context, filter repository.AdminUserFilter, limit, offset int) ([]*repository.AdminUserRow, int, error) {
	filter.BootstrapEmail = s.bootstrapEmail
	return s.admins.ListUsers(ctx, filter, limit, offset)
}

// PromoteBootstrap sets the admin flag on the verified, enabled ADMIN_EMAIL
// holder and records a bootstrap_admin audit entry, returning its id when the
// flag was newly set and uuid.Nil otherwise.
func (s *AdminRoleService) PromoteBootstrap(ctx context.Context) (uuid.UUID, error) {
	if s.bootstrapEmail == "" {
		return uuid.Nil, nil
	}
	id, err := s.admins.PromoteVerifiedByEmail(ctx, s.bootstrapEmail)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, err
	}
	s.recordBootstrap(ctx, id, s.bootstrapEmail)
	return id, nil
}

// PersistBootstrap sets the admin flag on user when it is the unflagged
// ADMIN_EMAIL holder, recording a bootstrap_admin audit entry only for the
// write that set it.
func (s *AdminRoleService) PersistBootstrap(ctx context.Context, user *models.User) error {
	if user.IsAdmin || !s.IsBootstrapAdmin(user) {
		return nil
	}
	result, err := s.admins.SetAdmin(ctx, user.ID)
	if err != nil {
		return err
	}
	if result == repository.GrantApplied {
		user.IsAdmin = true
		s.recordBootstrap(ctx, user.ID, user.Email)
	}
	return nil
}

func (s *AdminRoleService) recordBootstrap(ctx context.Context, userID uuid.UUID, email string) {
	details, _ := json.Marshal(map[string]any{"email": email, "source": "ADMIN_EMAIL"})
	if err := s.admins.InsertAuditLog(ctx, &repository.AdminAuditLogEntry{
		ID:           uuid.New(),
		AdminUserID:  userID,
		Action:       "bootstrap_admin",
		TargetUserID: &userID,
		Details:      details,
		CreatedAt:    time.Now(),
	}); err != nil {
		slog.Error("audit log: insert failed", "action", "bootstrap_admin", "admin_user_id", userID, "error", err)
	}
}

// Grant sets the admin flag on a verified, enabled account, reporting whether
// the flag was newly set.
func (s *AdminRoleService) Grant(ctx context.Context, targetID uuid.UUID) (bool, error) {
	result, err := s.admins.SetAdmin(ctx, targetID)
	if err != nil {
		return false, err
	}
	switch result {
	case repository.GrantApplied:
		return true, nil
	case repository.GrantAlreadyAdmin:
		return false, nil
	case repository.GrantNotFound:
		return false, ErrAdminTargetNotFound
	case repository.GrantUnverified:
		return false, ErrAdminTargetUnverified
	default:
		return false, ErrAdminTargetDisabled
	}
}

// Revoke clears the admin flag, reporting whether it was set.
func (s *AdminRoleService) Revoke(ctx context.Context, targetID uuid.UUID) (bool, error) {
	return guardResult(s.admins.RevokeAdmin(ctx, targetID, s.bootstrapEmail))
}

// Disable disables the account and clears its admin flag, reporting whether
// the flag was set.
func (s *AdminRoleService) Disable(ctx context.Context, targetID uuid.UUID) (bool, error) {
	return guardResult(s.admins.DisableUser(ctx, targetID, s.bootstrapEmail))
}

// Delete deletes the account, reporting whether it carried the admin flag.
func (s *AdminRoleService) Delete(ctx context.Context, targetID uuid.UUID) (bool, error) {
	return guardResult(s.admins.DeleteUser(ctx, targetID, s.bootstrapEmail))
}

func guardResult(change repository.AdminChange, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	switch change.Guard {
	case repository.AdminGuardNotFound:
		return false, ErrAdminTargetNotFound
	case repository.AdminGuardBootstrap:
		return false, ErrBootstrapAdmin
	case repository.AdminGuardLastAdmin:
		return false, ErrLastAdmin
	}
	return change.FlagCleared, nil
}
