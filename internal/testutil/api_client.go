package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type APITestClient struct {
	Router *gin.Engine
	T      *testing.T
	Token  string
}

func NewAPITestClient(t *testing.T, router *gin.Engine) *APITestClient {
	return &APITestClient{
		Router: router,
		T:      t,
	}
}

func (c *APITestClient) SetAuthToken(token string) {
	c.Token = token
}

func (c *APITestClient) POST(path string, body interface{}) *httptest.ResponseRecorder {
	return c.makeRequest("POST", path, body)
}

func (c *APITestClient) GET(path string) *httptest.ResponseRecorder {
	return c.makeRequest("GET", path, nil)
}

func (c *APITestClient) PUT(path string, body interface{}) *httptest.ResponseRecorder {
	return c.makeRequest("PUT", path, body)
}

func (c *APITestClient) PATCH(path string, body interface{}) *httptest.ResponseRecorder {
	return c.makeRequest("PATCH", path, body)
}

func (c *APITestClient) DELETE(path string) *httptest.ResponseRecorder {
	return c.makeRequest("DELETE", path, nil)
}

func (c *APITestClient) makeRequest(method, path string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			c.T.Fatalf("Failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewBuffer(jsonBytes)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		c.T.Fatalf("Failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	}

	w := httptest.NewRecorder()
	c.Router.ServeHTTP(w, req)

	return w
}

func (c *APITestClient) ParseJSON(w *httptest.ResponseRecorder, target interface{}) {
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		c.T.Fatalf("Failed to parse JSON response: %v", err)
	}
}
