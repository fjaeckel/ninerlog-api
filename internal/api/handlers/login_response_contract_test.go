package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fjaeckel/ninerlog-api/internal/api/generated"
	"github.com/getkin/kin-openapi/openapi3"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// POST /auth/login answers 200 with one of two bodies: the tokens, or — when the
// account has 2FA enabled — a challenge to complete at /auth/2fa/login. The spec
// described only the first for a long time, so generated clients (the iOS app
// among them) could not decode a 2FA login at all. These tests pin both shapes
// against the spec the clients are generated from.

// login200Schema returns the schema the spec gives for a 200 on /auth/login.
func login200Schema(t *testing.T) *openapi3.Schema {
	t.Helper()

	swagger, err := generated.GetSwagger()
	if err != nil {
		t.Fatalf("GetSwagger failed: %v", err)
	}
	path := swagger.Paths.Find("/auth/login")
	if path == nil || path.Post == nil {
		t.Fatal("POST /auth/login is missing from the spec")
	}
	resp := path.Post.Responses.Status(200)
	if resp == nil || resp.Value == nil {
		t.Fatal("POST /auth/login has no 200 response")
	}
	media := resp.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil {
		t.Fatal("POST /auth/login 200 has no application/json schema")
	}
	return media.Schema.Value
}

// asJSONValue round-trips through JSON so the value is validated exactly as it
// goes over the wire.
func asJSONValue(t *testing.T, v interface{}) interface{} {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	return decoded
}

func sampleAuthResponse() generated.AuthResponse {
	return generated.AuthResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		ExpiresIn:    900,
		User: generated.User{
			Id:        openapi_types.UUID{},
			Email:     openapi_types.Email("pilot@example.com"),
			Name:      "Test Pilot",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
}

func sampleTwoFactorChallenge() generated.TwoFactorLoginRequired {
	return generated.TwoFactorLoginRequired{
		RequiresTwoFactor: true,
		TwoFactorToken:    "two-factor-token",
	}
}

func TestLogin200AcceptsCompletedLogin(t *testing.T) {
	schema := login200Schema(t)

	if err := schema.VisitJSON(asJSONValue(t, sampleAuthResponse())); err != nil {
		t.Errorf("A completed login must validate against the 200 schema: %v", err)
	}
}

func TestLogin200AcceptsTwoFactorChallenge(t *testing.T) {
	schema := login200Schema(t)

	if err := schema.VisitJSON(asJSONValue(t, sampleTwoFactorChallenge())); err != nil {
		t.Errorf("The 2FA challenge must validate against the 200 schema: %v", err)
	}
}

// Asserts the two branches are mutually exclusive.
func TestLogin200BranchesAreUnambiguous(t *testing.T) {
	schema := login200Schema(t)
	if len(schema.OneOf) != 2 {
		t.Fatalf("Expected the 200 to be a oneOf over two shapes, got %d", len(schema.OneOf))
	}

	bodies := map[string]interface{}{
		"completed login": asJSONValue(t, sampleAuthResponse()),
		"2FA challenge":   asJSONValue(t, sampleTwoFactorChallenge()),
	}

	for name, body := range bodies {
		matches := 0
		for _, branch := range schema.OneOf {
			if err := branch.Value.VisitJSON(body); err == nil {
				matches++
			}
		}
		if matches != 1 {
			t.Errorf("Expected the %s body to match exactly one branch, matched %d", name, matches)
		}
	}
}
