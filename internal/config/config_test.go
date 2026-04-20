package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	os.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	os.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	defer func() {
		os.Unsetenv("JWT_ACCESS_SECRET")
		os.Unsetenv("JWT_REFRESH_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Server.Port != "8080" {
		t.Errorf("Server.Port = %s, want 8080", cfg.Server.Port)
	}
	if cfg.JWT.AccessSecret != "test-access-secret" {
		t.Errorf("JWT.AccessSecret = %s, want test-access-secret", cfg.JWT.AccessSecret)
	}
}

func TestLoadMissingSecrets(t *testing.T) {
	os.Unsetenv("JWT_ACCESS_SECRET")
	os.Unsetenv("JWT_REFRESH_SECRET")

	_, err := Load()
	if err == nil {
		t.Error("Load() should return error when secrets are missing")
	}
}

func TestDSN(t *testing.T) {
	dbConfig := DatabaseConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "postgres",
		Password: "password",
		DBName:   "testdb",
		SSLMode:  "disable",
	}

	expected := "host=localhost port=5432 user=postgres password=password dbname=testdb sslmode=disable"
	got := dbConfig.DSN()
	if got != expected {
		t.Errorf("DSN() = %s, want %s", got, expected)
	}
}

func TestGetDurationEnv(t *testing.T) {
	os.Setenv("TEST_DURATION", "60")
	defer os.Unsetenv("TEST_DURATION")

	duration := getDurationEnv("TEST_DURATION", 10*time.Second)
	if duration != 60*time.Second {
		t.Errorf("getDurationEnv() = %v, want 60s", duration)
	}

	duration = getDurationEnv("NOT_SET", 20*time.Second)
	if duration != 20*time.Second {
		t.Errorf("getDurationEnv() = %v, want 20s", duration)
	}
}

func TestGetDurationEnv_DurationString(t *testing.T) {
	t.Setenv("TEST_DUR_STR", "5m30s")

	duration := getDurationEnv("TEST_DUR_STR", 10*time.Second)
	if duration != 5*time.Minute+30*time.Second {
		t.Errorf("getDurationEnv() = %v, want 5m30s", duration)
	}
}

func TestGetDurationEnv_InvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("TEST_DUR_INVALID", "not-a-duration")

	duration := getDurationEnv("TEST_DUR_INVALID", 42*time.Second)
	if duration != 42*time.Second {
		t.Errorf("getDurationEnv(invalid) = %v, want 42s default", duration)
	}
}

func TestLoad_MissingOnlyRefreshSecret(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	os.Unsetenv("JWT_REFRESH_SECRET")

	_, err := Load()
	if err == nil {
		t.Error("Load() should error when only refresh secret is missing")
	}
}

func TestLoad_CustomServerPort(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	t.Setenv("SERVER_PORT", "3000")
	t.Setenv("SERVER_HOST", "127.0.0.1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Port != "3000" {
		t.Errorf("Server.Port = %q, want 3000", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Server.Host = %q, want 127.0.0.1", cfg.Server.Host)
	}
}

func TestLoad_CustomDatabaseConfig(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "ninerlog")
	t.Setenv("DB_PASSWORD", "secret123")
	t.Setenv("DB_NAME", "ninerlog_test")
	t.Setenv("DB_SSL_MODE", "require")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Database.Host = %q, want db.example.com", cfg.Database.Host)
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("Database.SSLMode = %q, want require", cfg.Database.SSLMode)
	}
}

func TestLoad_CustomJWTExpiry(t *testing.T) {
	t.Setenv("JWT_ACCESS_SECRET", "test-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")
	t.Setenv("JWT_ACCESS_EXPIRY", "300")
	t.Setenv("JWT_REFRESH_EXPIRY", "24h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.JWT.AccessTokenExpiry != 300*time.Second {
		t.Errorf("AccessTokenExpiry = %v, want 5m", cfg.JWT.AccessTokenExpiry)
	}
	if cfg.JWT.RefreshTokenExpiry != 24*time.Hour {
		t.Errorf("RefreshTokenExpiry = %v, want 24h", cfg.JWT.RefreshTokenExpiry)
	}
}
