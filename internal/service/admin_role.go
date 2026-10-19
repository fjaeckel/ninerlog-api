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

	// ErrLastAdmin is returned when revoking the last verified, enabled admin.
	ErrLastAdmin = errors.New("cannot revoke the last admin")

	// ErrBootstrapAdmin is returned when revoking the account that holds the
	// ADMIN_EMAIL address.
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

// AdminRoleService grants and revokes the per-account admin flag.
// bootstrapEmail is ADMIN_EMAIL; its verified holder is always an admin.
type AdminRoleService struct {
	users          repository.UserRepository
	admins         repository.AdminRepository
	bootstrapEmail string
}

func NewAdminRoleService(users repository.UserRepository, admins repository.AdminRepository, bootstrapEmail string) *AdminRoleService {
	return &AdminRoleService{users: users, admins: admins, bootstrapEmail: bootstrapEmail}
}

// BootstrapEmail returns the configured ADMIN_EMAIL.
func (s *AdminRoleService) BootstrapEmail() string { return s.bootstrapEmail }

// PromoteBootstrap sets the admin flag on the verified ADMIN_EMAIL holder and
// records a bootstrap_admin audit entry, returning its id when the flag was
// newly set and uuid.Nil otherwise.
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
// ADMIN_EMAIL holder and records a bootstrap_admin audit entry.
func (s *AdminRoleService) PersistBootstrap(ctx context.Context, user *models.User) error {
	if user.IsAdmin || !IsBootstrapAdmin(user, s.bootstrapEmail) {
		return nil
	}
	if err := s.admins.SetAdmin(ctx, user.ID); err != nil {
		return err
	}
	user.IsAdmin = true
	s.recordBootstrap(ctx, user.ID, user.Email)
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

// Grant sets the admin flag on the target account and returns it.
func (s *AdminRoleService) Grant(ctx context.Context, targetID uuid.UUID) (*models.User, error) {
	user, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAdminTargetNotFound
		}
		return nil, err
	}
	if !user.EmailVerified {
		return nil, ErrAdminTargetUnverified
	}
	if user.Disabled {
		return nil, ErrAdminTargetDisabled
	}
	if err := s.admins.SetAdmin(ctx, targetID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAdminTargetNotFound
		}
		return nil, err
	}
	user.IsAdmin = true
	return user, nil
}

// Revoke clears the admin flag on the target account and returns it.
func (s *AdminRoleService) Revoke(ctx context.Context, targetID uuid.UUID) (*models.User, error) {
	user, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrAdminTargetNotFound
		}
		return nil, err
	}
	if IsBootstrapAdmin(user, s.bootstrapEmail) {
		return nil, ErrBootstrapAdmin
	}
	if !user.IsAdmin {
		return user, nil
	}
	revoked, err := s.admins.RevokeAdmin(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if !revoked {
		return nil, ErrLastAdmin
	}
	user.IsAdmin = false
	return user, nil
}

// ReleaseForRemoval prepares a target account for disabling or deletion:
// it refuses the ADMIN_EMAIL holder and clears the admin flag unless no other
// verified, enabled admin would remain. It reports whether the flag was
// cleared.
func (s *AdminRoleService) ReleaseForRemoval(ctx context.Context, target *models.User) (bool, error) {
	if IsBootstrapAdmin(target, s.bootstrapEmail) {
		return false, ErrBootstrapAdmin
	}
	if !target.IsAdmin {
		return false, nil
	}
	revoked, err := s.admins.RevokeAdmin(ctx, target.ID)
	if err != nil {
		return false, err
	}
	if !revoked {
		return false, ErrLastAdmin
	}
	target.IsAdmin = false
	return true, nil
}
