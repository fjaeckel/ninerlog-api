package email

import (
	"os"
	"testing"
)

func TestLoadSMTPConfig_Defaults(t *testing.T) {
	os.Unsetenv("SMTP_HOST")
	os.Unsetenv("SMTP_PORT")
	os.Unsetenv("SMTP_USERNAME")
	os.Unsetenv("SMTP_PASSWORD")
	os.Unsetenv("SMTP_FROM")

	cfg := LoadSMTPConfig()
	if cfg.Host != "" {
		t.Errorf("Host = %q, want empty", cfg.Host)
	}
	if cfg.Port != "587" {
		t.Errorf("Port = %q, want 587", cfg.Port)
	}
	if cfg.Username != "" {
		t.Errorf("Username = %q, want empty", cfg.Username)
	}
	if cfg.From != "noreply@ninerlog.app" {
		t.Errorf("From = %q, want noreply@ninerlog.app", cfg.From)
	}
}

func TestLoadSMTPConfig_FromEnv(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_USERNAME", "user@example.com")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "admin@example.com")

	cfg := LoadSMTPConfig()
	if cfg.Host != "smtp.example.com" {
		t.Errorf("Host = %q, want smtp.example.com", cfg.Host)
	}
	if cfg.Port != "465" {
		t.Errorf("Port = %q, want 465", cfg.Port)
	}
	if cfg.Username != "user@example.com" {
		t.Errorf("Username = %q, want user@example.com", cfg.Username)
	}
}

func TestIsConfigured_NotConfigured(t *testing.T) {
	cfg := &SMTPConfig{}
	if cfg.IsConfigured() {
		t.Error("IsConfigured() = true for empty config")
	}
}

func TestIsConfigured_FullyConfigured(t *testing.T) {
	cfg := &SMTPConfig{Host: "smtp.example.com", Username: "user"}
	if !cfg.IsConfigured() {
		t.Error("IsConfigured() = false for configured SMTP")
	}
}

func TestIsConfigured_OnlyHost(t *testing.T) {
	// Host-only is valid — supports test SMTP servers (MailPit) without auth
	cfg := &SMTPConfig{Host: "smtp.example.com"}
	if !cfg.IsConfigured() {
		t.Error("IsConfigured() = false with host set (host-only is valid for auth-less SMTP)")
	}
}

func TestIsConfigured_OnlyUsername(t *testing.T) {
	// Username without host is NOT valid
	cfg := &SMTPConfig{Username: "user"}
	if cfg.IsConfigured() {
		t.Error("IsConfigured() = true with only username set (no host)")
	}
}

func TestNewSender(t *testing.T) {
	cfg := &SMTPConfig{Host: "smtp.example.com", Port: "587"}
	sender := NewSender(cfg)
	if sender == nil {
		t.Error("NewSender() returned nil")
	}
}

func TestSend_DryRun(t *testing.T) {
	cfg := &SMTPConfig{}
	sender := NewSender(cfg)
	err := sender.Send("test@example.com", "Test Subject", "<p>Hello</p>")
	if err != nil {
		t.Errorf("Send() dry-run error = %v, want nil", err)
	}
}

func TestGetEnv_WithValue(t *testing.T) {
	t.Setenv("TEST_SMTP_VAR", "custom_value")
	val := getEnv("TEST_SMTP_VAR", "default")
	if val != "custom_value" {
		t.Errorf("getEnv() = %q, want custom_value", val)
	}
}

func TestGetEnv_WithDefault(t *testing.T) {
	os.Unsetenv("TEST_SMTP_MISSING")
	val := getEnv("TEST_SMTP_MISSING", "default_val")
	if val != "default_val" {
		t.Errorf("getEnv() = %q, want default_val", val)
	}
}

func TestSanitizeHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"clean value", "user@example.com", "user@example.com"},
		{"strips CR", "user@example.com\rBcc: attacker@evil.com", "user@example.comBcc: attacker@evil.com"},
		{"strips LF", "user@example.com\nBcc: attacker@evil.com", "user@example.comBcc: attacker@evil.com"},
		{"strips CRLF", "user@example.com\r\nBcc: attacker@evil.com", "user@example.comBcc: attacker@evil.com"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeHeader(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSend_DryRun_SanitizesHeaders(t *testing.T) {
	cfg := &SMTPConfig{}
	sender := NewSender(cfg)
	// Should not error even with injection attempt (dry-run strips CRLF)
	err := sender.Send("test@example.com\r\nBcc: evil@example.com", "Subject\r\nBcc: evil", "<p>Hello</p>")
	if err != nil {
		t.Errorf("Send() dry-run with injection attempt error = %v, want nil", err)
	}
}
