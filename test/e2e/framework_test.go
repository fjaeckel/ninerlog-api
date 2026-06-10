//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

var baseURL string
var mailpitURL string

func init() {
	baseURL = os.Getenv("E2E_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3333"
	}
	mailpitURL = os.Getenv("E2E_MAILPIT_URL")
	if mailpitURL == "" {
		mailpitURL = "http://localhost:8025"
	}
}

type E2EClient struct {
	t      *testing.T
	client *http.Client
	token  string
}

func NewE2EClient(t *testing.T) *E2EClient {
	return &E2EClient{
		t:      t,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *E2EClient) SetToken(token string) { c.token = token }
func (c *E2EClient) ClearToken()           { c.token = "" }

type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

func (r *Response) JSON(target interface{}) error {
	return json.Unmarshal(r.Body, target)
}

func (c *E2EClient) Do(method, path string, body interface{}) *Response {
	c.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("Failed to marshal body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	url := baseURL + "/api/v1" + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		c.t.Fatalf("Failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return &Response{StatusCode: resp.StatusCode, Body: respBody, Headers: resp.Header}
}

func (c *E2EClient) GET(path string) *Response                     { return c.Do("GET", path, nil) }
func (c *E2EClient) POST(path string, body interface{}) *Response  { return c.Do("POST", path, body) }
func (c *E2EClient) PUT(path string, body interface{}) *Response   { return c.Do("PUT", path, body) }
func (c *E2EClient) PATCH(path string, body interface{}) *Response { return c.Do("PATCH", path, body) }
func (c *E2EClient) DELETE(path string) *Response                  { return c.Do("DELETE", path, nil) }
func (c *E2EClient) DELETEWithBody(path string, body interface{}) *Response {
	return c.Do("DELETE", path, body)
}

func (c *E2EClient) DoRaw(method, url string, body interface{}) *Response {
	c.t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, baseURL+url, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("Raw request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return &Response{StatusCode: resp.StatusCode, Body: respBody, Headers: resp.Header}
}

func assertStatus(t *testing.T, resp *Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Errorf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(resp.Body))
	}
}

func requireStatus(t *testing.T, resp *Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		t.Fatalf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(resp.Body))
	}
}

type AuthResponseBody struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	User         struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"user"`
	RequiresTwoFactor bool   `json:"requiresTwoFactor"`
	TwoFactorToken    string `json:"twoFactorToken"`
}

func registerUser(t *testing.T, c *E2EClient, email, password, name string) AuthResponseBody {
	t.Helper()
	resp := c.POST("/auth/register", map[string]string{"email": email, "password": password, "name": name})
	requireStatus(t, resp, http.StatusCreated)

	// Email verification is now required: pull the verification token from
	// the e2e SMTP server (mailpit) and exchange it for an AuthResponse.
	token := extractVerificationToken(t, email)
	verifyResp := c.POST("/auth/verify-email", map[string]string{"token": token})
	requireStatus(t, verifyResp, http.StatusOK)
	var auth AuthResponseBody
	verifyResp.JSON(&auth)
	return auth
}

// extractVerificationToken polls mailpit for the verification email sent to
// `email` and returns the token query parameter from the verification link.
func extractVerificationToken(t *testing.T, email string) string {
	t.Helper()
	return extractVerificationTokenSubject(t, email, "Confirm your email")
}

// extractVerificationTokenSubject is like extractVerificationToken but allows
// matching a localized subject line (the verification email is sent in the
// user's preferredLocale).
func extractVerificationTokenSubject(t *testing.T, email, subjectContains string) string {
	t.Helper()
	msg := mailpitRequireEmail(t, email, subjectContains)
	re := regexp.MustCompile(`token=([A-Za-z0-9_\-=]+)`)
	matches := re.FindStringSubmatch(msg.HTML)
	if len(matches) < 2 {
		matches = re.FindStringSubmatch(msg.Text)
	}
	if len(matches) < 2 {
		t.Fatalf("Could not extract verification token from email body")
	}
	return matches[1]
}

func loginUser(t *testing.T, c *E2EClient, email, password string) AuthResponseBody {
	t.Helper()
	resp := c.POST("/auth/login", map[string]string{"email": email, "password": password})
	requireStatus(t, resp, http.StatusOK)
	var auth AuthResponseBody
	resp.JSON(&auth)
	return auth
}

func registerAndLogin(t *testing.T, c *E2EClient, email, password, name string) AuthResponseBody {
	t.Helper()
	auth := registerUser(t, c, email, password, name)
	c.SetToken(auth.AccessToken)
	return auth
}

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@e2e-test.com", prefix, time.Now().UnixNano())
}

func today() string              { return time.Now().Format("2006-01-02") }
func pastDate(days int) string   { return time.Now().AddDate(0, 0, -days).Format("2006-01-02") }
func futureDate(days int) string { return time.Now().AddDate(0, 0, days).Format("2006-01-02") }

// MailPit API helpers — query the test SMTP server

// MailPitMessage represents a single email in MailPit
type MailPitMessage struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
	From    struct {
		Address string `json:"Address"`
		Name    string `json:"Name"`
	} `json:"From"`
	To []struct {
		Address string `json:"Address"`
		Name    string `json:"Name"`
	} `json:"To"`
	Snippet string `json:"Snippet"`
}

// MailPitSearchResult is the response from MailPit's messages/search API.
// Note: `total` is total messages in the entire mailbox, NOT search results.
// Use `messages_count` for the number of messages matching the current query.
type MailPitSearchResult struct {
	Total         int              `json:"total"`          // total in mailbox (all messages)
	MessagesCount int              `json:"messages_count"` // matching current query
	Messages      []MailPitMessage `json:"messages"`
}

// mailpitDeleteAll clears all messages from MailPit
func mailpitDeleteAll(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("DELETE", mailpitURL+"/api/v1/messages", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to clear MailPit: %v", err)
	}
	defer resp.Body.Close()
}

// mailpitSearchByRecipient searches MailPit for messages to a specific email address.
//
// The query intentionally does NOT use a `to:` prefix. For security (CWE-640),
// the API delivers the recipient via the SMTP envelope only and omits the `To:`
// header from the message bytes (see pkg/email/smtp.go). MailPit therefore
// records the envelope-only recipient as Bcc, so a `to:` search would never
// match. A bare address query matches the recipient regardless of which address
// header MailPit assigns it to.
func mailpitSearchByRecipient(t *testing.T, email string) MailPitSearchResult {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	searchURL := fmt.Sprintf("%s/api/v1/search?query=%s", mailpitURL, url.QueryEscape(email))
	resp, err := client.Get(searchURL)
	if err != nil {
		t.Fatalf("Failed to query MailPit: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result MailPitSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse MailPit response: %v (body: %s)", err, string(body))
	}
	return result
}

// mailpitGetMessages returns all messages from MailPit
func mailpitGetMessages(t *testing.T) MailPitSearchResult {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(mailpitURL + "/api/v1/messages")
	if err != nil {
		t.Fatalf("Failed to query MailPit messages: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result MailPitSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to parse MailPit response: %v (body: %s)", err, string(body))
	}
	return result
}

// MailPitFullMessage is the full message from GET /api/v1/message/{ID}
type MailPitFullMessage struct {
	ID      string `json:"ID"`
	Subject string `json:"Subject"`
	From    struct {
		Address string `json:"Address"`
		Name    string `json:"Name"`
	} `json:"From"`
	To []struct {
		Address string `json:"Address"`
		Name    string `json:"Name"`
	} `json:"To"`
	// Bcc carries the recipient when the API delivers it via the SMTP envelope
	// only (security fix for CWE-640 — see pkg/email/smtp.go). MailPit records an
	// envelope-only recipient here with a null To header.
	Bcc []struct {
		Address string `json:"Address"`
		Name    string `json:"Name"`
	} `json:"Bcc"`
	HTML string `json:"HTML"`
	Text string `json:"Text"`
}

// recipientAddresses returns every address the message was delivered to,
// combining the To and Bcc headers. The recipient may appear in either: the API
// omits the To header and delivers via the SMTP envelope (CWE-640), which MailPit
// surfaces as Bcc.
func (m MailPitFullMessage) recipientAddresses() []string {
	addrs := make([]string, 0, len(m.To)+len(m.Bcc))
	for _, t := range m.To {
		addrs = append(addrs, t.Address)
	}
	for _, b := range m.Bcc {
		addrs = append(addrs, b.Address)
	}
	return addrs
}


// mailpitGetMessage retrieves the full message including HTML body
func mailpitGetMessage(t *testing.T, messageID string) MailPitFullMessage {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/v1/message/%s", mailpitURL, messageID))
	if err != nil {
		t.Fatalf("Failed to get MailPit message %s: %v", messageID, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var msg MailPitFullMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("Failed to parse MailPit message: %v (body: %s)", err, string(body))
	}
	return msg
}

// mailpitFindEmail searches for a message to a recipient with a subject substring, returns the full message
func mailpitFindEmail(t *testing.T, recipientEmail, subjectContains string) *MailPitFullMessage {
	t.Helper()
	result := mailpitSearchByRecipient(t, recipientEmail)
	for _, msg := range result.Messages {
		if strings.Contains(msg.Subject, subjectContains) {
			full := mailpitGetMessage(t, msg.ID)
			return &full
		}
	}
	return nil
}

// mailpitRequireEmail searches for a message and fatals if not found
func mailpitRequireEmail(t *testing.T, recipientEmail, subjectContains string) MailPitFullMessage {
	t.Helper()
	msg := mailpitFindEmail(t, recipientEmail, subjectContains)
	if msg == nil {
		// List what was found for debugging
		result := mailpitSearchByRecipient(t, recipientEmail)
		subjects := make([]string, len(result.Messages))
		for i, m := range result.Messages {
			subjects[i] = m.Subject
		}
		t.Fatalf("No email to %s with subject containing %q found. Got %d emails: %v",
			recipientEmail, subjectContains, result.MessagesCount, subjects)
	}
	return *msg
}

func TestHealthCheck(t *testing.T) {
	c := NewE2EClient(t)
	resp := c.DoRaw("GET", "/health", nil)
	requireStatus(t, resp, http.StatusOK)
	var body map[string]string
	resp.JSON(&body)
	if body["status"] != "ok" {
		t.Errorf("Expected status ok, got %s", body["status"])
	}
}
