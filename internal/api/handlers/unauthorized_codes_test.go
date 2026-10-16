package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/fjaeckel/ninerlog-api/internal/api/errcode"
	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Every 401 carries a code, and clients branch on it: session_invalid means
// refresh and replay, any other code means a credential in the request was
// refused and the session must survive. These tests pin both the codes and the
// sentences, since older clients still match the sentences.

// call runs a handler with a JSON body, authenticated when userID is non-nil,
// and decodes the error envelope.
func call(t *testing.T, handler func(*gin.Context), body string, userID *uuid.UUID) (int, generated.Error) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if userID != nil {
		c.Set("userID", *userID)
	}
	c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler(c)
	var e generated.Error
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	return w.Code, e
}

func assert401(t *testing.T, status int, e generated.Error, wantCode, wantMessage string) {
	t.Helper()
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if e.Code == nil || *e.Code != wantCode {
		got := "<nil>"
		if e.Code != nil {
			got = *e.Code
		}
		t.Errorf("code = %s, want %s", got, wantCode)
	}
	if e.Error != wantMessage {
		t.Errorf("error = %q, want %q", e.Error, wantMessage)
	}
}

// registeredUser registers a local account and returns its ID.
func registeredUser(t *testing.T, h *APIHandler, repo *mockUserRepo, email string) uuid.UUID {
	t.Helper()
	status, _ := call(t, h.RegisterUser, `{"email":"`+email+`","password":"Password1234!","name":"P"}`, nil)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("register: %d", status)
	}
	u, err := repo.GetByEmail(context.Background(), email)
	if err != nil || u == nil {
		t.Fatalf("registered user not found: %v", err)
	}
	return u.ID
}

func TestUnauthorizedCodes_CredentialRefusals(t *testing.T) {
	h, repo := setupTestHandler()
	id := registeredUser(t, h, repo, "codes@example.com")

	t.Run("login", func(t *testing.T) {
		s, e := call(t, h.LoginUser, `{"email":"codes@example.com","password":"wrong-Password1!"}`, nil)
		assert401(t, s, e, errcode.InvalidCredentials, "Invalid credentials")
	})
	t.Run("change password", func(t *testing.T) {
		s, e := call(t, h.ChangePassword, `{"currentPassword":"wrong-Password1!","newPassword":"NewPassword1234!"}`, &id)
		assert401(t, s, e, errcode.InvalidCredentials, "Current password is incorrect")
	})
	t.Run("change email", func(t *testing.T) {
		s, e := call(t, h.UpdateCurrentUser, `{"email":"other@example.com","currentPassword":"wrong-Password1!"}`, &id)
		assert401(t, s, e, errcode.InvalidCredentials, "Password is incorrect")
	})
	t.Run("delete account", func(t *testing.T) {
		s, e := call(t, h.DeleteCurrentUser, `{"password":"wrong-Password1!"}`, &id)
		assert401(t, s, e, errcode.InvalidCredentials, "Password is incorrect")
	})
	t.Run("disable 2FA", func(t *testing.T) {
		u, _ := repo.GetByID(context.Background(), id)
		u.TwoFactorEnabled = true
		s, e := call(t, h.Disable2FA, `{"password":"wrong-Password1!"}`, &id)
		assert401(t, s, e, errcode.InvalidCredentials, "Password is incorrect")
	})
	t.Run("refresh token", func(t *testing.T) {
		s, e := call(t, h.RefreshToken, `{"refreshToken":"not-a-token"}`, nil)
		assert401(t, s, e, errcode.InvalidRefreshToken, "Invalid or expired refresh token")
	})
	t.Run("2FA challenge token", func(t *testing.T) {
		s, e := call(t, h.Login2FA, `{"twoFactorToken":"not-a-token","code":"123456"}`, nil)
		assert401(t, s, e, errcode.InvalidTwoFactorToken, "Invalid or expired 2FA token. Please login again.")
	})
}

func TestUnauthorizedCodes_SessionRefusalFromHandler(t *testing.T) {
	h, _ := setupTestHandler()
	s, e := call(t, h.GetCurrentUser, ``, nil)
	assert401(t, s, e, errcode.SessionInvalid, "Unauthorized")
}

// The codes are a published contract; renaming one breaks every client.
func TestUnauthorizedCodes_Values(t *testing.T) {
	want := map[string]string{
		errcode.SessionInvalid:             "session_invalid",
		errcode.InvalidCredentials:         "invalid_credentials",
		errcode.InvalidRefreshToken:        "invalid_refresh_token",
		errcode.InvalidSignInCode:          "invalid_sign_in_code",
		errcode.InvalidTwoFactorToken:      "invalid_two_factor_token",
		errcode.TwoFactorRequired:          "two_factor_required",
		errcode.InvalidTwoFactorCode:       "invalid_two_factor_code",
		errcode.InvalidProviderCredentials: "invalid_provider_credentials",
	}
	for got, w := range want {
		if got != w {
			t.Errorf("code %q changed, want %q", got, w)
		}
	}
}

// TestUnauthorizedCodes_Inventory reads the handler sources and pins every 401
// to its code and sentence, covering the paths that need services the unit
// harness does not build (passkeys, OIDC, cloud backup). A plain sendError 401
// is sent as session_invalid, so a new credential refusal written that way
// would make clients sign the user out over a mistyped password; this test
// fails until it is given a code here and in the spec.
func TestUnauthorizedCodes_Inventory(t *testing.T) {
	coded := map[string]string{
		"Invalid credentials":                               errcode.InvalidCredentials,
		"Current password is incorrect":                     errcode.InvalidCredentials,
		"Password is incorrect":                             errcode.InvalidCredentials,
		"Unknown passkey":                                   errcode.InvalidCredentials,
		"Passkey verification failed":                       errcode.InvalidCredentials,
		"Invalid or expired refresh token":                  errcode.InvalidRefreshToken,
		"Sign-in code is invalid or has expired":            errcode.InvalidSignInCode,
		"Invalid or expired 2FA token. Please login again.": errcode.InvalidTwoFactorToken,
		"Invalid 2FA code":                                  errcode.InvalidTwoFactorCode,
		"Invalid two-factor code":                           errcode.InvalidTwoFactorCode,
		"This account uses two-factor authentication. Enter a code from your authenticator app or one of your recovery codes to reset the password.": errcode.TwoFactorRequired,
		"Invalid provider credentials": errcode.InvalidProviderCredentials,
	}
	codeNames := map[string]string{
		"InvalidCredentials":         errcode.InvalidCredentials,
		"InvalidRefreshToken":        errcode.InvalidRefreshToken,
		"InvalidSignInCode":          errcode.InvalidSignInCode,
		"InvalidTwoFactorToken":      errcode.InvalidTwoFactorToken,
		"InvalidTwoFactorCode":       errcode.InvalidTwoFactorCode,
		"TwoFactorRequired":          errcode.TwoFactorRequired,
		"InvalidProviderCredentials": errcode.InvalidProviderCredentials,
	}
	sessionMessages := map[string]bool{"Unauthorized": true, "Authentication required": true}

	plain := regexp.MustCompile(`sendError\(c, http\.StatusUnauthorized, "([^"]*)"`)
	withCode := regexp.MustCompile(`sendCodedError\(c, http\.StatusUnauthorized, errcode\.(\w+),\s*"([^"]*)"`)
	anyUnauthorized := regexp.MustCompile(`StatusUnauthorized`)

	files, _ := filepath.Glob("*.go")
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(src)
		total := len(anyUnauthorized.FindAllStringIndex(s, -1))
		matched := 0
		for _, m := range plain.FindAllStringSubmatch(s, -1) {
			matched++
			if !sessionMessages[m[1]] {
				t.Errorf("%s: 401 %q is sent as session_invalid; give it a credential code", f, m[1])
			}
		}
		for _, m := range withCode.FindAllStringSubmatch(s, -1) {
			matched++
			seen[m[2]] = true
			want, known := coded[m[2]]
			if !known {
				t.Errorf("%s: coded 401 %q is not in the inventory", f, m[2])
				continue
			}
			if codeNames[m[1]] != want {
				t.Errorf("%s: 401 %q has errcode.%s, want %s", f, m[2], m[1], want)
			}
		}
		if f == "api_handler.go" {
			total-- // sendError's own default
		}
		if matched != total {
			t.Errorf("%s: %d StatusUnauthorized uses, %d recognised — send every 401 through sendError or sendCodedError", f, total, matched)
		}
	}
	for msg := range coded {
		if !seen[msg] {
			t.Errorf("inventory sentence %q no longer sent by any handler", msg)
		}
	}
}
