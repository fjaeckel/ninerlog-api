package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

type TestDBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func DefaultTestDBConfig() TestDBConfig {
	return TestDBConfig{
		Host:     getEnv("TEST_DB_HOST", "localhost"),
		Port:     getEnv("TEST_DB_PORT", "5433"),
		User:     getEnv("TEST_DB_USER", "testuser"),
		Password: getEnv("TEST_DB_PASSWORD", "testpass"),
		DBName:   getEnv("TEST_DB_NAME", "ninerlog_test"),
	}
}

func SetupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	config := DefaultTestDBConfig()
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.Host, config.Port, config.User, config.Password, config.DBName,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Failed to ping test database after retries: %v", ctx.Err())
		case <-ticker.C:
			if err := db.PingContext(ctx); err == nil {
				goto connected
			}
		}
	}

connected:
	CleanupTestDB(t, db)
	return db
}

func CleanupTestDB(t *testing.T, db *sql.DB) {
	t.Helper()

	tables := []string{
		"password_reset_tokens",
		"refresh_tokens",
		"users",
	}

	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			t.Logf("Warning: Failed to truncate %s: %v", table, err)
		}
	}
}

func TeardownTestDB(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Errorf("Failed to close test database: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
