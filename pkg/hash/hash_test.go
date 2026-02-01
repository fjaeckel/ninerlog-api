package hash

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hashed == "" {
		t.Error("HashPassword() returned empty hash")
	}
	if !strings.HasPrefix(hashed, "$2a$") && !strings.HasPrefix(hashed, "$2b$") {
		t.Error("HashPassword() returned invalid bcrypt hash")
	}
}

func TestComparePassword(t *testing.T) {
	password := "testpassword123"
	hashed, _ := HashPassword(password)

	if err := ComparePassword(hashed, password); err != nil {
		t.Error("ComparePassword() failed for correct password")
	}

	if err := ComparePassword(hashed, "wrongpassword"); err == nil {
		t.Error("ComparePassword() should fail for incorrect password")
	}
}

func TestHashToken(t *testing.T) {
	token := "test-token-123"
	hash := HashToken(token)

	if len(hash) != 64 {
		t.Errorf("HashToken() length = %d, want 64", len(hash))
	}

	if hash == token {
		t.Error("HashToken() returned plaintext token")
	}

	hash2 := HashToken(token)
	if hash != hash2 {
		t.Error("HashToken() not deterministic")
	}
}
