package handlers

import (
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/google/uuid"
)

func createTestUser(email, name string) *models.User {
	return &models.User{
		ID:    uuid.New(),
		Email: email,
		Name:  name,
		// Admin status requires a verified address (see isAdminUser); real
		// accounts that reach admin checks have completed verification.
		EmailVerified: true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
}

func TestIsAdminUser(t *testing.T) {
	tests := []struct {
		name       string
		adminEmail string
		userEmail  string
		flagged    bool
		want       bool
	}{
		{
			name:      "flagged account without admin email",
			userEmail: "second-admin@example.com",
			flagged:   true,
			want:      true,
		},
		{
			name:       "flagged account alongside admin email",
			adminEmail: "admin@example.com",
			userEmail:  "second-admin@example.com",
			flagged:    true,
			want:       true,
		},
		{
			name:       "matching email",
			adminEmail: "admin@example.com",
			userEmail:  "admin@example.com",
			want:       true,
		},
		{
			name:       "case insensitive match",
			adminEmail: "Admin@Example.COM",
			userEmail:  "admin@example.com",
			want:       true,
		},
		{
			name:       "non-matching email",
			adminEmail: "admin@example.com",
			userEmail:  "user@example.com",
			want:       false,
		},
		{
			name:       "empty admin email",
			adminEmail: "",
			userEmail:  "admin@example.com",
			want:       false,
		},
		{
			name:       "empty user email",
			adminEmail: "admin@example.com",
			userEmail:  "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &APIHandler{adminRoles: service.NewAdminRoleService(nil, tt.adminEmail)}
			user := &models.User{Email: tt.userEmail, EmailVerified: true, IsAdmin: tt.flagged}
			if got := h.isAdminUser(user); got != tt.want {
				t.Errorf("isAdminUser(%q) = %v, want %v", tt.userEmail, got, tt.want)
			}
		})
	}
}

func TestBuildUserResponse_IncludesIsAdmin(t *testing.T) {
	// Create a minimal handler with admin email set
	h := &APIHandler{adminRoles: service.NewAdminRoleService(nil, "admin@ninerlog.app")}

	t.Run("admin user", func(t *testing.T) {
		user := createTestUser("admin@ninerlog.app", "Admin User")
		resp := h.buildUserResponse(user)
		if resp.IsAdmin == nil || !*resp.IsAdmin {
			t.Error("Expected isAdmin=true for admin user")
		}
	})

	t.Run("regular user", func(t *testing.T) {
		user := createTestUser("pilot@example.com", "Regular Pilot")
		resp := h.buildUserResponse(user)
		if resp.IsAdmin == nil || *resp.IsAdmin {
			t.Error("Expected isAdmin=false for regular user")
		}
	})

	t.Run("no admin configured", func(t *testing.T) {
		h2 := &APIHandler{adminRoles: service.NewAdminRoleService(nil, "")}
		user := createTestUser("admin@ninerlog.app", "Admin User")
		resp := h2.buildUserResponse(user)
		if resp.IsAdmin == nil || *resp.IsAdmin {
			t.Error("Expected isAdmin=false when no admin email configured")
		}
	})
}
