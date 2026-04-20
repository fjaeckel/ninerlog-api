package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/models"
	"github.com/fjaeckel/ninerlog-api/internal/repository"
	"github.com/fjaeckel/ninerlog-api/internal/service"
	"github.com/fjaeckel/ninerlog-api/pkg/hash"
	"github.com/fjaeckel/ninerlog-api/pkg/jwt"
	"github.com/google/uuid"
)

// Mock repositories for testing
type mockUserRepo struct {
	users map[string]*models.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users: make(map[string]*models.User),
	}
}

func (m *mockUserRepo) Create(ctx context.Context, user *models.User) error {
	if _, exists := m.users[user.Email]; exists {
		return service.ErrUserAlreadyExists
	}
	user.ID = uuid.New()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	user, exists := m.users[email]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return user, nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	for _, user := range m.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (m *mockUserRepo) Update(ctx context.Context, user *models.User) error {
	for _, u := range m.users {
		if u.ID == user.ID {
			m.users[user.Email] = user
			return nil
		}
	}
	return repository.ErrNotFound
}

func (m *mockUserRepo) Delete(ctx context.Context, id uuid.UUID) error {
	for email, user := range m.users {
		if user.ID == id {
			delete(m.users, email)
			return nil
		}
	}
	return repository.ErrNotFound
}

func (m *mockUserRepo) IncrementFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockUserRepo) ResetFailedLoginAttempts(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockUserRepo) LockAccount(ctx context.Context, id uuid.UUID, until time.Time) error {
	return nil
}

type mockRefreshTokenRepo struct {
	tokens map[string]*models.RefreshToken
}

func newMockRefreshTokenRepo() *mockRefreshTokenRepo {
	return &mockRefreshTokenRepo{
		tokens: make(map[string]*models.RefreshToken),
	}
}

func (m *mockRefreshTokenRepo) Create(ctx context.Context, token *models.RefreshToken) error {
	token.ID = uuid.New()
	token.CreatedAt = time.Now()
	token.UpdatedAt = time.Now()
	m.tokens[token.TokenHash] = token
	return nil
}

func (m *mockRefreshTokenRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	token, exists := m.tokens[tokenHash]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return token, nil
}

func (m *mockRefreshTokenRepo) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	token, exists := m.tokens[tokenHash]
	if !exists {
		return repository.ErrNotFound
	}
	token.Revoked = true
	return nil
}

func (m *mockRefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	for _, token := range m.tokens {
		if token.UserID == userID {
			token.Revoked = true
		}
	}
	return nil
}

func (m *mockRefreshTokenRepo) DeleteForUser(ctx context.Context, userID uuid.UUID) error {
	for hash, token := range m.tokens {
		if token.UserID == userID {
			delete(m.tokens, hash)
		}
	}
	return nil
}

func (m *mockRefreshTokenRepo) DeleteExpired(ctx context.Context) error {
	now := time.Now()
	for hash, token := range m.tokens {
		if token.ExpiresAt.Before(now) {
			delete(m.tokens, hash)
		}
	}
	return nil
}

type mockPasswordResetRepo struct {
	tokens map[string]*models.PasswordResetToken
}

func newMockPasswordResetRepo() *mockPasswordResetRepo {
	return &mockPasswordResetRepo{
		tokens: make(map[string]*models.PasswordResetToken),
	}
}

func (m *mockPasswordResetRepo) Create(ctx context.Context, token *models.PasswordResetToken) error {
	token.ID = uuid.New()
	token.CreatedAt = time.Now()
	m.tokens[token.TokenHash] = token
	return nil
}

func (m *mockPasswordResetRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*models.PasswordResetToken, error) {
	token, exists := m.tokens[tokenHash]
	if !exists {
		return nil, repository.ErrNotFound
	}
	return token, nil
}

func (m *mockPasswordResetRepo) MarkAsUsed(ctx context.Context, tokenHash string) error {
	token, exists := m.tokens[tokenHash]
	if !exists {
		return repository.ErrNotFound
	}
	token.Used = true
	return nil
}

func (m *mockPasswordResetRepo) DeleteExpired(ctx context.Context) error {
	now := time.Now()
	for hash, token := range m.tokens {
		if token.ExpiresAt.Before(now) {
			delete(m.tokens, hash)
		}
	}
	return nil
}

func (m *mockPasswordResetRepo) DeleteForUser(ctx context.Context, userID uuid.UUID) error {
	for hash, token := range m.tokens {
		if token.UserID == userID {
			delete(m.tokens, hash)
		}
	}
	return nil
}

// Test functions
func setupAuthService() *service.AuthService {
	userRepo := newMockUserRepo()
	refreshTokenRepo := newMockRefreshTokenRepo()
	passwordResetRepo := newMockPasswordResetRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	return service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
}

func TestRegister(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	user, tokens, err := authService.Register(ctx, input)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if user.Email != input.Email {
		t.Errorf("Expected email %s, got %s", input.Email, user.Email)
	}

	if tokens.AccessToken == "" {
		t.Error("Expected access token to be generated")
	}

	if tokens.RefreshToken == "" {
		t.Error("Expected refresh token to be generated")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	// First registration should succeed
	_, _, err := authService.Register(ctx, input)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Second registration with same email should fail
	_, _, err = authService.Register(ctx, input)
	if err != service.ErrUserAlreadyExists {
		t.Errorf("Expected ErrUserAlreadyExists, got %v", err)
	}
}

func TestLogin(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	// Register user first
	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	// Test login
	loginInput := service.LoginInput{
		Email:    "test@example.com",
		Password: "password1234",
	}

	user, tokens, err := authService.Login(ctx, loginInput)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if user.Email != loginInput.Email {
		t.Errorf("Expected email %s, got %s", loginInput.Email, user.Email)
	}

	if tokens.AccessToken == "" {
		t.Error("Expected access token to be generated")
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	// Register user first
	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	// Test login with wrong password
	loginInput := service.LoginInput{
		Email:    "test@example.com",
		Password: "wrongpassword1",
	}

	_, _, err = authService.Login(ctx, loginInput)
	if err != service.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}

func TestPasswordHashing(t *testing.T) {
	password := "testpassword123"

	// Hash password
	hashed, err := hash.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Verify correct password
	err = hash.ComparePassword(hashed, password)
	if err != nil {
		t.Error("Failed to verify correct password")
	}

	// Verify incorrect password
	err = hash.ComparePassword(hashed, "wrongpassword1")
	if err == nil {
		t.Error("Should have failed to verify incorrect password")
	}
}

func TestRefreshToken(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, tokens, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	newTokens, err := authService.RefreshToken(ctx, tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}

	if newTokens.AccessToken == "" || newTokens.RefreshToken == "" {
		t.Error("Expected new tokens")
	}
}

func TestRefreshTokenInvalid(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	_, err := authService.RefreshToken(ctx, "invalid-token")
	if err != service.ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken, got %v", err)
	}
}

func TestRefreshTokenRevoked(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, tokens, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	if err := authService.Logout(ctx, tokens.RefreshToken); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	_, err = authService.RefreshToken(ctx, tokens.RefreshToken)
	if err != service.ErrTokenRevoked {
		t.Errorf("Expected ErrTokenRevoked, got %v", err)
	}
}

func TestLogout(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, tokens, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	err = authService.Logout(ctx, tokens.RefreshToken)
	if err != nil {
		t.Errorf("Logout failed: %v", err)
	}
}

func TestRequestPasswordReset(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	resetToken, err := authService.RequestPasswordReset(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}

	if resetToken == "" {
		t.Error("Expected reset token")
	}
}

func TestRequestPasswordResetNonExistentUser(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	resetToken, err := authService.RequestPasswordReset(ctx, "nonexistent@example.com")
	if err != nil {
		t.Errorf("RequestPasswordReset should not error for non-existent user: %v", err)
	}

	if resetToken != "" {
		t.Error("Should not return token for non-existent user")
	}
}

func TestResetPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	resetToken, err := authService.RequestPasswordReset(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}

	err = authService.ResetPassword(ctx, resetToken, "newpassword456")
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	loginInput := service.LoginInput{
		Email:    "test@example.com",
		Password: "newpassword456",
	}

	_, _, err = authService.Login(ctx, loginInput)
	if err != nil {
		t.Errorf("Login with new password failed: %v", err)
	}
}

func TestResetPasswordInvalidToken(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	err := authService.ResetPassword(ctx, "invalid-token", "newpassword")
	if err != service.ErrInvalidToken {
		t.Errorf("Expected ErrInvalidToken, got %v", err)
	}
}

func TestResetPasswordUsedToken(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	registerInput := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}

	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	resetToken, err := authService.RequestPasswordReset(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset failed: %v", err)
	}

	err = authService.ResetPassword(ctx, resetToken, "newpassword456")
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	err = authService.ResetPassword(ctx, resetToken, "anotherpassword")
	if err != service.ErrTokenUsed {
		t.Errorf("Expected ErrTokenUsed, got %v", err)
	}
}

func TestChangePassword(t *testing.T) {
	userRepo := newMockUserRepo()
	refreshTokenRepo := newMockRefreshTokenRepo()
	passwordResetRepo := newMockPasswordResetRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)

	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
	ctx := context.Background()

	// Register a user
	registerInput := service.RegisterInput{
		Email:    "change@example.com",
		Password: "oldpassword123",
		Name:     "Change Test",
	}
	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	// Get user to find ID
	user, _ := userRepo.GetByEmail(ctx, "change@example.com")

	// Change password with correct current password
	err = authService.ChangePassword(ctx, user.ID, "oldpassword123", "newpassword456")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Verify new password works
	err = hash.ComparePassword(user.PasswordHash, "newpassword456")
	if err != nil {
		t.Error("New password doesn't match after change")
	}

	// Change password with wrong current password should fail
	err = authService.ChangePassword(ctx, user.ID, "wrongpassword1", "another123")
	if err != service.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}

	// Change to short password should fail
	err = authService.ChangePassword(ctx, user.ID, "newpassword456", "short")
	if err == nil {
		t.Error("Expected error for short password, got nil")
	}
}

func TestDeleteUser(t *testing.T) {
	userRepo := newMockUserRepo()
	refreshTokenRepo := newMockRefreshTokenRepo()
	passwordResetRepo := newMockPasswordResetRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)

	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
	ctx := context.Background()

	// Register a user
	registerInput := service.RegisterInput{
		Email:    "delete@example.com",
		Password: "password1234",
		Name:     "Delete Test",
	}
	_, _, err := authService.Register(ctx, registerInput)
	if err != nil {
		t.Fatalf("Registration failed: %v", err)
	}

	user, _ := userRepo.GetByEmail(ctx, "delete@example.com")
	userID := user.ID

	// Delete with wrong password should fail
	err = authService.DeleteUser(ctx, userID, "wrongpassword1")
	if err != service.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}

	// Delete with correct password should succeed
	err = authService.DeleteUser(ctx, userID, "password1234")
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user is gone
	_, err = authService.GetUserByID(ctx, userID)
	if err == nil {
		t.Error("Expected error when getting deleted user, got nil")
	}
}

func TestRegister_EmptyEmail(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "",
		Password: "password1234",
		Name:     "Test User",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrEmailRequired {
		t.Errorf("Expected ErrEmailRequired, got %v", err)
	}
}

func TestRegister_EmptyPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "",
		Name:     "Test User",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrPasswordRequired {
		t.Errorf("Expected ErrPasswordRequired, got %v", err)
	}
}

func TestRegister_EmptyName(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrNameRequired {
		t.Errorf("Expected ErrNameRequired, got %v", err)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "short",
		Name:     "Test User",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrPasswordTooShort {
		t.Errorf("Expected ErrPasswordTooShort, got %v", err)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "not-an-email",
		Password: "password1234",
		Name:     "Test User",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrInvalidEmail {
		t.Errorf("Expected ErrInvalidEmail, got %v", err)
	}
}

func TestLogin_NonexistentUser(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	loginInput := service.LoginInput{
		Email:    "nobody@example.com",
		Password: "password1234",
	}
	_, _, err := authService.Login(ctx, loginInput)
	if err != service.ErrInvalidCredentials {
		t.Errorf("Expected ErrInvalidCredentials, got %v", err)
	}
}

func TestGetUserByID(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}
	user, _, _ := authService.Register(ctx, input)

	got, err := authService.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID() error = %v", err)
	}
	if got.Email != "test@example.com" {
		t.Errorf("Email = %q, want test@example.com", got.Email)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	_, err := authService.GetUserByID(ctx, uuid.New())
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

func TestUpdateUser(t *testing.T) {
	userRepo := newMockUserRepo()
	refreshTokenRepo := newMockRefreshTokenRepo()
	passwordResetRepo := newMockPasswordResetRepo()
	jwtManager := jwt.NewManager("test-secret", "test-refresh-secret", 15*time.Minute, 7*24*time.Hour)
	authService := service.NewAuthService(userRepo, refreshTokenRepo, passwordResetRepo, jwtManager)
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}
	user, _, _ := authService.Register(ctx, input)

	user.Name = "Updated Name"
	err := authService.UpdateUser(ctx, user)
	if err != nil {
		t.Fatalf("UpdateUser() error = %v", err)
	}

	got, _ := authService.GetUserByID(ctx, user.ID)
	if got.Name != "Updated Name" {
		t.Errorf("Name = %q, want Updated Name", got.Name)
	}
}

func TestGenerateTokensForUser(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}
	user, _, _ := authService.Register(ctx, input)

	tokens, err := authService.GenerateTokensForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GenerateTokensForUser() error = %v", err)
	}
	if tokens.AccessToken == "" {
		t.Error("Expected non-empty access token")
	}
	if tokens.RefreshToken == "" {
		t.Error("Expected non-empty refresh token")
	}
}

func TestRegister_LongPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	longPassword := make([]byte, 73)
	for i := range longPassword {
		longPassword[i] = 'a'
	}
	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: string(longPassword),
		Name:     "Test User",
	}
	_, _, err := authService.Register(ctx, input)
	if err != service.ErrPasswordTooLong {
		t.Errorf("Expected ErrPasswordTooLong, got %v", err)
	}
}

func TestRegister_EmailNormalization(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "  Test@Example.COM  ",
		Password: "password1234",
		Name:     "Test User",
	}
	user, _, err := authService.Register(ctx, input)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %q, want 'test@example.com' (normalized)", user.Email)
	}
}

func TestLogin_EmailNormalization(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}
	_, _, _ = authService.Register(ctx, input)

	// Login with different casing should work
	loginInput := service.LoginInput{
		Email:    "  TEST@EXAMPLE.COM  ",
		Password: "password1234",
	}
	user, _, err := authService.Login(ctx, loginInput)
	if err != nil {
		t.Fatalf("Login with different case failed: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Email = %q, want test@example.com", user.Email)
	}
}

func TestResetPassword_ShortPassword(t *testing.T) {
	authService := setupAuthService()
	ctx := context.Background()

	input := service.RegisterInput{
		Email:    "test@example.com",
		Password: "password1234",
		Name:     "Test User",
	}
	_, _, _ = authService.Register(ctx, input)

	resetToken, _ := authService.RequestPasswordReset(ctx, "test@example.com")

	err := authService.ResetPassword(ctx, resetToken, "short")
	if err != service.ErrPasswordTooShort {
		t.Errorf("Expected ErrPasswordTooShort, got %v", err)
	}
}
