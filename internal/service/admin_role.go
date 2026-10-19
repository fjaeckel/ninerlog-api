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

// IsAdmin reports whether the user is an admin: a verified, enabled account
// that carries the admin flag or holds bootstrapEmail (case-insensitive).
func IsAdmin(user *models.User, bootstrapEmail string) bool {
	if user == nil || !user.EmailVerified || user.Disabled {
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

// Delete deletes target on behalf of actorID. The delete_user entry, and a
// revoke_admin entry when target carried the flag, are written in the same
// transaction before the row goes.
func (s *AdminRoleService) Delete(ctx context.Context, actorID uuid.UUID, target *models.User) error {
	audit := func(flagCleared bool) []*repository.AdminAuditLogEntry {
		now := time.Now()
		entry := func(action string, details map[string]any) *repository.AdminAuditLogEntry {
			payload, _ := json.Marshal(details)
			return &repository.AdminAuditLogEntry{
				ID: uuid.New(), AdminUserID: actorID, Action: action,
				TargetUserID: &target.ID, Details: payload, CreatedAt: now,
			}
		}
		var out []*repository.AdminAuditLogEntry
		if flagCleared {
			out = append(out, entry("revoke_admin", map[string]any{"email": target.Email, "reason": "delete_user"}))
		}
		return append(out, entry("delete_user", map[string]any{"email": target.Email, "name": target.Name}))
	}
	_, err := guardResult(s.admins.DeleteUser(ctx, target.ID, s.bootstrapEmail, audit))
	return err
}

// CheckResetTwoFactor returns ErrBootstrapAdmin when actorID may not reset
// target's second factor: target is the ADMIN_EMAIL holder and not actorID.
func (s *AdminRoleService) CheckResetTwoFactor(actorID uuid.UUID, target *models.User) error {
	if target.ID != actorID && s.IsBootstrapAdmin(target) {
		return ErrBootstrapAdmin
	}
	return nil
}

// ChangeOwnEmail changes an admin's own address under the last-admin guard
// and reports whether it did; for a non-admin it does nothing and returns
// false. Returns ErrLastAdmin when no other active admin would remain, and
// ErrUserAlreadyExists when the address is taken.
func (s *AdminRoleService) ChangeOwnEmail(ctx context.Context, user *models.User, email string) (bool, error) {
	if !s.IsAdmin(user) {
		return false, nil
	}
	change, err := s.admins.ChangeEmail(ctx, user.ID, email, s.bootstrapEmail)
	if errors.Is(err, repository.ErrDuplicateEmail) {
		return false, ErrUserAlreadyExists
	}
	if _, err := guardResult(change, err); err != nil {
		return false, err
	}
	user.Email = email
	user.EmailVerified = false
	return true, nil
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
