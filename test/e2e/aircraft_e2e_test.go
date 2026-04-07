//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestAircraftCRUD(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("aircraft"), "SecurePass123!", "AC User")
	var acID string

	t.Run("create aircraft", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EABC", "type": "C172", "make": "Cessna", "model": "172S",
			"aircraftClass": "SEP_LAND",
		})
		requireStatus(t, resp, http.StatusCreated)
		var ac map[string]interface{}
		resp.JSON(&ac)
		acID = ac["id"].(string)
	})

	t.Run("create with all fields", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EMEP", "type": "PA44", "make": "Piper", "model": "Seminole",
			"aircraftClass": "MEP_LAND", "isComplex": true, "isHighPerformance": true,
			"isTailwheel": false, "notes": "Club MEP",
		})
		requireStatus(t, resp, http.StatusCreated)
	})

	t.Run("duplicate registration returns 409", func(t *testing.T) {
		resp := c.POST("/aircraft", map[string]interface{}{
			"registration": "D-EABC", "type": "PA28", "make": "Piper", "model": "Cherokee",
		})
		assertStatus(t, resp, http.StatusConflict)
	})

	t.Run("list with pagination", func(t *testing.T) {
		resp := c.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		data := r["data"].([]interface{})
		if len(data) < 2 {
			t.Errorf("Expected >=2, got %d", len(data))
		}
	})

	t.Run("get by id", func(t *testing.T) {
		requireStatus(t, c.GET(fmt.Sprintf("/aircraft/%s", acID)), http.StatusOK)
	})

	t.Run("update", func(t *testing.T) {
		resp := c.PATCH(fmt.Sprintf("/aircraft/%s", acID), map[string]interface{}{"notes": "Updated"})
		requireStatus(t, resp, http.StatusOK)
	})

	t.Run("delete", func(t *testing.T) {
		assertStatus(t, c.DELETE(fmt.Sprintf("/aircraft/%s", acID)), http.StatusNoContent)
		assertStatus(t, c.GET(fmt.Sprintf("/aircraft/%s", acID)), http.StatusNotFound)
	})

	t.Run("nonexistent returns 404", func(t *testing.T) {
		assertStatus(t, c.GET("/aircraft/00000000-0000-0000-0000-000000000000"), http.StatusNotFound)
	})

	t.Run("missing fields returns 400", func(t *testing.T) {
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{"registration": "X"}), http.StatusBadRequest)
	})

	t.Run("no auth returns 401", func(t *testing.T) {
		c.ClearToken()
		assertStatus(t, c.POST("/aircraft", map[string]interface{}{
			"registration": "X", "type": "C172", "make": "Cessna", "model": "172",
		}), http.StatusUnauthorized)
	})
}

func TestAircraftIsolation(t *testing.T) {
	c1 := NewE2EClient(t)
	c2 := NewE2EClient(t)
	registerAndLogin(t, c1, uniqueEmail("ac-iso1"), "SecurePass123!", "U1")
	registerAndLogin(t, c2, uniqueEmail("ac-iso2"), "SecurePass123!", "U2")

	resp := c1.POST("/aircraft", map[string]interface{}{
		"registration": "D-EISO", "type": "C172", "make": "Cessna", "model": "172",
	})
	requireStatus(t, resp, http.StatusCreated)
	var ac map[string]interface{}
	resp.JSON(&ac)
	aid := ac["id"].(string)

	t.Run("user2 cannot see", func(t *testing.T) {
		resp := c2.GET("/aircraft")
		requireStatus(t, resp, http.StatusOK)
		var r map[string]interface{}
		resp.JSON(&r)
		data := r["data"].([]interface{})
		if len(data) != 0 {
			t.Errorf("Expected 0, got %d", len(data))
		}
	})

	t.Run("user2 cannot access by id", func(t *testing.T) {
		assertStatus(t, c2.GET(fmt.Sprintf("/aircraft/%s", aid)), http.StatusNotFound)
	})

	t.Run("different users same registration ok", func(t *testing.T) {
		requireStatus(t, c2.POST("/aircraft", map[string]interface{}{
			"registration": "D-EISO", "type": "PA28", "make": "Piper", "model": "Cherokee",
		}), http.StatusCreated)
	})
}
