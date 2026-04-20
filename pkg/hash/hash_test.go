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

func TestHashPassword_UniquePerCall(t *testing.T) {
	password := "samepassword123"
	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	if hash1 == hash2 {
		t.Error("Two bcrypt hashes of same password should differ (unique salts)")
	}
}

func TestComparePassword_EmptyHash(t *testing.T) {
	err := ComparePassword("", "password")
	if err == nil {
		t.Error("ComparePassword with empty hash should fail")
	}
}

func TestHashToken_EmptyInput(t *testing.T) {
	hash := HashToken("")
	if len(hash) != 64 {
		t.Errorf("HashToken('') length = %d, want 64", len(hash))
	}
}

func TestHashToken_DifferentInputsDifferentHashes(t *testing.T) {
	h1 := HashToken("token-a")
	h2 := HashToken("token-b")
	if h1 == h2 {
		t.Error("Different tokens should produce different hashes")
	}
}

func TestComparePassword_CorrectAfterHash(t *testing.T) {
	passwords := []string{
		"simplepassword",
		"P@$$w0rd!Complex#123",
		"unicode-пароль-密码",
		strings.Repeat("a", 72), // max bcrypt length
	}
	for _, pw := range passwords {
		hashed, err := HashPassword(pw)
		if err != nil {
			t.Fatalf("HashPassword(%q) error = %v", pw, err)
		}
		if err := ComparePassword(hashed, pw); err != nil {
			t.Errorf("ComparePassword failed for %q: %v", pw, err)
		}
	}
}
