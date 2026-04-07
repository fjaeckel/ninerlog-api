//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var baseURL string

func init() {
	baseURL = os.Getenv("E2E_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:3333"
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
func (c *E2EClient) ClearToken()            { c.token = "" }

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

func (c *E2EClient) GET(path string) *Response                        { return c.Do("GET", path, nil) }
func (c *E2EClient) POST(path string, body interface{}) *Response     { return c.Do("POST", path, body) }
func (c *E2EClient) PUT(path string, body interface{}) *Response      { return c.Do("PUT", path, body) }
func (c *E2EClient) PATCH(path string, body interface{}) *Response    { return c.Do("PATCH", path, body) }
func (c *E2EClient) DELETE(path string) *Response                     { return c.Do("DELETE", path, nil) }
func (c *E2EClient) DELETEWithBody(path string, body interface{}) *Response { return c.Do("DELETE", path, body) }

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
	var auth AuthResponseBody
	resp.JSON(&auth)
	return auth
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

func today() string                { return time.Now().Format("2006-01-02") }
func pastDate(days int) string     { return time.Now().AddDate(0, 0, -days).Format("2006-01-02") }
func futureDate(days int) string   { return time.Now().AddDate(0, 0, days).Format("2006-01-02") }

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
