package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCredentialIsExpired_NoExpiryDate(t *testing.T) {
	c := &Credential{ID: uuid.New(), ExpiryDate: nil}
	if c.IsExpired() {
		t.Error("IsExpired() = true, want false for nil expiry")
	}
}

func TestCredentialIsExpired_FutureDate(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour)
	c := &Credential{ID: uuid.New(), ExpiryDate: &future}
	if c.IsExpired() {
		t.Error("IsExpired() = true, want false for future date")
	}
}

func TestCredentialIsExpired_PastDate(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	c := &Credential{ID: uuid.New(), ExpiryDate: &past}
	if !c.IsExpired() {
		t.Error("IsExpired() = false, want true for past date")
	}
}

func TestCredentialIsExpiringSoon_NoExpiryDate(t *testing.T) {
	c := &Credential{ID: uuid.New(), ExpiryDate: nil}
	if c.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon() = true, want false for nil expiry")
	}
}

func TestCredentialIsExpiringSoon_ExpiresSoon(t *testing.T) {
	soon := time.Now().Add(15 * 24 * time.Hour)
	c := &Credential{ID: uuid.New(), ExpiryDate: &soon}
	if !c.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon(30) = false, want true for 15 days out")
	}
}

func TestCredentialIsExpiringSoon_NotExpiringSoon(t *testing.T) {
	far := time.Now().Add(60 * 24 * time.Hour)
	c := &Credential{ID: uuid.New(), ExpiryDate: &far}
	if c.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon(30) = true, want false for 60 days out")
	}
}

func TestCredentialIsExpiringSoon_AlreadyExpired(t *testing.T) {
	past := time.Now().Add(-5 * 24 * time.Hour)
	c := &Credential{ID: uuid.New(), ExpiryDate: &past}
	if c.IsExpiringSoon(30) {
		t.Error("IsExpiringSoon() = true, want false for already expired")
	}
}

func TestCredentialTypes(t *testing.T) {
	types := []CredentialType{
		CredentialTypeEASAClass1Medical,
		CredentialTypeEASAClass2Medical,
		CredentialTypeEASALAPLMedical,
		CredentialTypeFAAClass1Medical,
		CredentialTypeFAAClass2Medical,
		CredentialTypeFAAClass3Medical,
		CredentialTypeLangICAOLevel4,
		CredentialTypeLangICAOLevel5,
		CredentialTypeLangICAOLevel6,
		CredentialTypeSecClearanceZUP,
		CredentialTypeSecClearanceZUBB,
		CredentialTypeOther,
	}
	for _, ct := range types {
		if ct == "" {
			t.Errorf("Credential type constant is empty")
		}
	}
}
