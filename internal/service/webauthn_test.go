package service

import (
	"testing"
)

func TestNewWebAuthnService_RequiresConfig(t *testing.T) {
	cases := []struct {
		name    string
		rpID    string
		rpName  string
		origins []string
	}{
		{"missing rp id", "", "NinerLog", []string{"https://example.com"}},
		{"missing rp name", "example.com", "", []string{"https://example.com"}},
		{"missing origins", "example.com", "NinerLog", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewWebAuthnService(tc.rpID, tc.rpName, tc.origins, nil, nil, nil, nil)
			if err == nil || svc != nil {
				t.Fatalf("expected error and nil service, got %v / %v", err, svc)
			}
		})
	}
}

func TestNewWebAuthnService_Valid(t *testing.T) {
	svc, err := NewWebAuthnService(
		"localhost",
		"NinerLog",
		[]string{"http://localhost:5173"},
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.wa == nil {
		t.Fatal("expected webauthn instance to be initialized")
	}
}
