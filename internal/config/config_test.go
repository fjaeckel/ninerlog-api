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
