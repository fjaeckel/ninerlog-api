//go:build e2e

package e2e_test

import (
	"net/http"
	"testing"
)

// Custom currency rules are stored server-side, so they must be reachable by
// any signed-in client on any device — and, since they are now declared in the
// OpenAPI spec, by every generated client rather than only the hand-written
// web one. These tests drive the endpoints over real HTTP the way a generated
// client would.

// nightLandingsRule is a valid definition: three landings at night in 90 days.
func nightLandingsRule() map[string]interface{} {
	return map[string]interface{}{
		"window": map[string]interface{}{"amount": 90, "unit": "days"},
		"filters": []map[string]interface{}{
			{"field": "has_night", "op": "is_true"},
		},
		"requirements": []map[string]interface{}{
			{"metric": "night_landings", "min": 3, "label": "3 night landings"},
		},
	}
}

// createCustomRule creates a rule and returns its id.
func createCustomRule(t *testing.T, c *E2EClient, name string) string {
	t.Helper()
	resp := c.POST("/custom-currency", map[string]interface{}{
		"name":       name,
		"emoji":      "🌙",
		"definition": nightLandingsRule(),
	})
	requireStatus(t, resp, http.StatusCreated)

	var created map[string]interface{}
	if err := resp.JSON(&created); err != nil {
		t.Fatalf("decode created rule: %v", err)
	}
	rule, ok := created["rule"].(map[string]interface{})
	if !ok {
		t.Fatalf("response has no rule object: %v", created)
	}
	id, ok := rule["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created rule has no id: %v", rule)
	}
	if _, ok := created["evaluation"].(map[string]interface{}); !ok {
		t.Errorf("created rule carries no evaluation: %v", created)
	}
	return id
}

func TestCustomCurrencyRuleLifecycle(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ccrule"), "SecurePass123!", "CC Rule")

	id := createCustomRule(t, c, "Night landings")

	// List carries the rule with its evaluation.
	resp := c.GET("/custom-currency")
	requireStatus(t, resp, http.StatusOK)
	var list []map[string]interface{}
	if err := resp.JSON(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(list))
	}

	// Get one.
	resp = c.GET("/custom-currency/" + id)
	requireStatus(t, resp, http.StatusOK)

	// Replace it.
	resp = c.PUT("/custom-currency/"+id, map[string]interface{}{
		"name":       "Night landings (revised)",
		"definition": nightLandingsRule(),
	})
	requireStatus(t, resp, http.StatusOK)
	var updated map[string]interface{}
	if err := resp.JSON(&updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if name := updated["rule"].(map[string]interface{})["name"]; name != "Night landings (revised)" {
		t.Errorf("name not updated: %v", name)
	}

	// Pause and resume.
	resp = c.PUT("/custom-currency/"+id+"/enabled", map[string]interface{}{"enabled": false})
	requireStatus(t, resp, http.StatusOK)
	var paused map[string]interface{}
	if err := resp.JSON(&paused); err != nil {
		t.Fatalf("decode paused: %v", err)
	}
	if enabled := paused["rule"].(map[string]interface{})["enabled"]; enabled != false {
		t.Errorf("rule should be paused, got enabled=%v", enabled)
	}
	requireStatus(t, c.PUT("/custom-currency/"+id+"/enabled",
		map[string]interface{}{"enabled": true}), http.StatusOK)

	// Opt into expiry mail.
	resp = c.PUT("/custom-currency/"+id+"/notify", map[string]interface{}{"notify": true})
	requireStatus(t, resp, http.StatusOK)
	var notified map[string]interface{}
	if err := resp.JSON(&notified); err != nil {
		t.Fatalf("decode notify: %v", err)
	}
	if notify := notified["rule"].(map[string]interface{})["notify"]; notify != true {
		t.Errorf("rule should notify, got %v", notify)
	}

	// Delete, then it is gone.
	requireStatus(t, c.DELETE("/custom-currency/"+id), http.StatusNoContent)
	requireStatus(t, c.GET("/custom-currency/"+id), http.StatusNotFound)
}

func TestCustomCurrencyPreviewDoesNotPersist(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ccprev"), "SecurePass123!", "CC Preview")

	resp := c.POST("/custom-currency/preview", map[string]interface{}{
		"definition": nightLandingsRule(),
	})
	requireStatus(t, resp, http.StatusOK)

	var evaluation map[string]interface{}
	if err := resp.JSON(&evaluation); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if _, ok := evaluation["status"]; !ok {
		t.Errorf("preview carries no status: %v", evaluation)
	}
	if _, ok := evaluation["requirements"]; !ok {
		t.Errorf("preview carries no requirements: %v", evaluation)
	}

	resp = c.GET("/custom-currency")
	requireStatus(t, resp, http.StatusOK)
	var list []map[string]interface{}
	if err := resp.JSON(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("preview persisted %d rule(s)", len(list))
	}
}

func TestCustomCurrencyRejectsInvalidDefinition(t *testing.T) {
	c := NewE2EClient(t)
	registerAndLogin(t, c, uniqueEmail("ccinvalid"), "SecurePass123!", "CC Invalid")

	cases := []struct {
		name       string
		definition map[string]interface{}
	}{
		{"unknown metric", map[string]interface{}{
			"window":       map[string]interface{}{"amount": 90, "unit": "days"},
			"requirements": []map[string]interface{}{{"metric": "barrel_rolls", "min": 1}},
		}},
		{"invalid window unit", map[string]interface{}{
			"window":       map[string]interface{}{"amount": 90, "unit": "fortnights"},
			"requirements": []map[string]interface{}{{"metric": "landings", "min": 1}},
		}},
		{"no requirements", map[string]interface{}{
			"window":       map[string]interface{}{"amount": 90, "unit": "days"},
			"requirements": []map[string]interface{}{},
		}},
		{"unit on a count metric", map[string]interface{}{
			"window":       map[string]interface{}{"amount": 90, "unit": "days"},
			"requirements": []map[string]interface{}{{"metric": "landings", "min": 1, "unit": "hours"}},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := c.POST("/custom-currency", map[string]interface{}{
				"name":       "Bad rule",
				"definition": tc.definition,
			})
			requireStatus(t, resp, http.StatusBadRequest)
		})
	}

	// A malformed id is rejected by the generated wrapper, not the service.
	requireStatus(t, c.GET("/custom-currency/not-a-uuid"), http.StatusBadRequest)
}

func TestCustomCurrencyRulesArePrivateToTheirOwner(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("ccowner"), "SecurePass123!", "CC Owner")
	id := createCustomRule(t, owner, "Owner's rule")

	other := NewE2EClient(t)
	registerAndLogin(t, other, uniqueEmail("ccother"), "SecurePass123!", "CC Other")

	// Another account must not see it by id, by list, or by mutation.
	requireStatus(t, other.GET("/custom-currency/"+id), http.StatusNotFound)
	requireStatus(t, other.DELETE("/custom-currency/"+id), http.StatusNotFound)

	resp := other.GET("/custom-currency")
	requireStatus(t, resp, http.StatusOK)
	var list []map[string]interface{}
	if err := resp.JSON(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("another account sees %d rule(s)", len(list))
	}

	// Unauthenticated access is refused.
	anon := NewE2EClient(t)
	requireStatus(t, anon.GET("/custom-currency"), http.StatusUnauthorized)
}

func TestCustomCurrencyShareAndImport(t *testing.T) {
	owner := NewE2EClient(t)
	registerAndLogin(t, owner, uniqueEmail("ccshare"), "SecurePass123!", "CC Share")
	id := createCustomRule(t, owner, "Shared night landings")

	// Before sharing, the rule has no token.
	resp := owner.POST("/custom-currency/"+id+"/share", nil)
	requireStatus(t, resp, http.StatusOK)
	var shared map[string]interface{}
	if err := resp.JSON(&shared); err != nil {
		t.Fatalf("decode share: %v", err)
	}
	token, ok := shared["shareToken"].(string)
	if !ok || token == "" {
		t.Fatalf("sharing produced no token: %v", shared)
	}
	if isShared := shared["isShared"]; isShared != true {
		t.Errorf("isShared = %v, want true", isShared)
	}

	// Another user reads the shared projection — without owner identity.
	importer := NewE2EClient(t)
	registerAndLogin(t, importer, uniqueEmail("ccimport"), "SecurePass123!", "CC Import")

	resp = importer.GET("/custom-currency/shared/" + token)
	requireStatus(t, resp, http.StatusOK)
	var view map[string]interface{}
	if err := resp.JSON(&view); err != nil {
		t.Fatalf("decode shared view: %v", err)
	}
	if view["name"] != "Shared night landings" {
		t.Errorf("shared name = %v", view["name"])
	}
	if _, leaked := view["userId"]; leaked {
		t.Error("shared view leaks the owner's user id")
	}

	// Importing copies it into the importer's account.
	resp = importer.POST("/custom-currency/shared/"+token+"/import", nil)
	requireStatus(t, resp, http.StatusCreated)
	var imported map[string]interface{}
	if err := resp.JSON(&imported); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	copyRule := imported["rule"].(map[string]interface{})
	if copyRule["id"] == id {
		t.Error("import reused the source rule's id")
	}
	if copyRule["importedFrom"] != id {
		t.Errorf("importedFrom = %v, want %s", copyRule["importedFrom"], id)
	}
	if copyRule["isShared"] != false {
		t.Errorf("an imported copy must not itself be shared: %v", copyRule["isShared"])
	}

	// Disabling sharing stops serving the token; the copy survives.
	requireStatus(t, owner.DELETE("/custom-currency/"+id+"/share"), http.StatusOK)
	requireStatus(t, importer.GET("/custom-currency/shared/"+token), http.StatusNotFound)

	resp = importer.GET("/custom-currency")
	requireStatus(t, resp, http.StatusOK)
	var list []map[string]interface{}
	if err := resp.JSON(&list); err != nil {
		t.Fatalf("decode importer list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("importer holds %d rule(s), want 1", len(list))
	}
}
