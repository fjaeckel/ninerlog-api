//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestContactCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("contact"), "SecurePass123!", "Contact")
	var cid string

	t.Run("create with all fields", func(t *testing.T) {
		resp := c.POST("/contacts", map[string]interface{}{
			"name": "CFI Johnson", "email": "j@fs.com", "phone": "+49-123", "notes": "Chief instructor",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ct map[string]interface{}
		resp.JSON(&ct)
		cid = ct["id"].(string)
	})

	t.Run("create name only", func(t *testing.T) {
		requireStatus(t, c.POST("/contacts", map[string]interface{}{"name": "Mike"}), http.StatusCreated)
	})

	t.Run("list contacts", func(t *testing.T) {
		resp := c.GET("/contacts")
		requireStatus(t, resp, http.StatusOK)
		var cts []interface{}
		resp.JSON(&cts)
		if len(cts) < 2 {
			t.Errorf("Expected >=2, got %d", len(cts))
		}
	})

	t.Run("search by name", func(t *testing.T) {
		resp := c.GET("/contacts/search?q=Johnson")
		requireStatus(t, resp, http.StatusOK)
		var cts []interface{}
		resp.JSON(&cts)
		if len(cts) < 1 {
			t.Error("Expected match for Johnson")
		}
	})

	t.Run("search no match", func(t *testing.T) {
		resp := c.GET("/contacts/search?q=ZZZZZ")
		requireStatus(t, resp, http.StatusOK)
		var cts []interface{}
		resp.JSON(&cts)
		if len(cts) != 0 {
			t.Errorf("Expected 0, got %d", len(cts))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/contacts/%s", cid)), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		resp := c.PUT(fmt.Sprintf("/contacts/%s", cid), map[string]interface{}{
			"name": "CFI Johnson Sr.", "email": "sr@fs.com",
		})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/contacts/%s", cid)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/contacts/%s", cid)), http.StatusNotFound)
	})

	t.Run("nonexistent returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/contacts/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("empty name returns 400", func(t *testing.T) {
		assertStatus(t, c.POST("/contacts", map[string]interface{}{"name": ""}), http.StatusBadRequest)
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/contacts", map[string]interface{}{"name": "X"}), http.StatusUnauthorized)
	})
}

func TestContactIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("cnt-iso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("cnt-iso2"), "SecurePass123!", "U2")

	resp := c1.POST("/contacts", map[string]interface{}{"name": "Private"})
	requireStatus(t, resp, http.StatusCreated)
	var ct map[string]interface{}
	resp.JSON(&ct)
	cid := ct["id"].(string)

	t.Run("user2 cannot see", func(t *testing.T) {
		resp := c2.GET("/contacts")
		requireStatus(t, resp, http.StatusOK)
		var cts []interface{}
		resp.JSON(&cts)
		if len(cts) != 0 {
			t.Errorf("Expected 0, got %d", len(cts))
		}
	})

	t.Run("user2 cannot access by id", func(t *testing.T) {
		assertStatus(t, c2.GET(fmt.Sprintf("/contacts/%s", cid)), http.StatusNotFound)
	})

	t.Run("user2 cannot search", func(t *testing.T) {
		resp := c2.GET("/contacts/search?q=Private")
		requireStatus(t, resp, http.StatusOK)
		var cts []interface{}
		resp.JSON(&cts)
		if len(cts) != 0 {
			t.Errorf("Expected 0, got %d", len(cts))
		}
	})
}
