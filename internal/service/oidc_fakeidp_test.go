package service_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// fakeIDP is a minimal but standards-shaped OpenID Connect provider: discovery
// document, JWKS, and a token endpoint that mints RS256 ID tokens.
//
// It exists so the whole login flow — discovery, code exchange, signature and
// nonce verification, provisioning — can be exercised in a unit test without a
// real identity provider, and so the negative cases (wrong key, wrong
// audience, wrong nonce) can be produced deliberately.
type fakeIDP struct {
	t      *testing.T
	server *httptest.Server
	key    *rsa.PrivateKey

	// Knobs the tests turn to produce invalid tokens.
	signWith      *rsa.PrivateKey // defaults to key
	audience      string          // defaults to the client ID
	nonceOverride string          // when set, replaces the nonce echoed back
	claims        map[string]any  // merged into the ID token
	expiry        time.Duration   // defaults to +1h

	// lastForm records what the client posted to the token endpoint, so tests
	// can assert PKCE and grant parameters were sent.
	lastForm url.Values
}

const fakeIDPKeyID = "test-key-1"

func newFakeIDP(t *testing.T, clientID string) *fakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	idp := &fakeIDP{t: t, key: key, audience: clientID, expiry: time.Hour}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                idp.issuer(),
			"authorization_endpoint":                idp.issuer() + "/authorize",
			"token_endpoint":                        idp.issuer() + "/token",
			"jwks_uri":                              idp.issuer() + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"scopes_supported":                      []string{"openid", "profile", "email"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := idp.key.Public().(*rsa.PublicKey)
		writeJSON(w, map[string]any{"keys": []map[string]any{{
			"kty": "RSA",
			"alg": "RS256",
			"use": "sig",
			"kid": fakeIDPKeyID,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		idp.lastForm = r.PostForm
		if r.PostForm.Get("code") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idp.mintIDToken(r.PostForm.Get("code")),
		})
	})

	idp.server = httptest.NewServer(mux)
	t.Cleanup(idp.server.Close)
	return idp
}

func (f *fakeIDP) issuer() string {
	if f.server == nil {
		// Discovery is fetched lazily; the issuer is only needed once the
		// server is up. Tests never call this before newFakeIDP returns.
		return ""
	}
	return f.server.URL
}

// nonceFor is set by the test from the authorization URL the service produced,
// mirroring how a real provider echoes the nonce it was given.
func (f *fakeIDP) mintIDToken(_ string) string {
	f.t.Helper()
	now := time.Now()
	claims := jwtlib.MapClaims{
		"iss": f.issuer(),
		"aud": f.audience,
		"sub": "subject-1",
		"iat": now.Unix(),
		"exp": now.Add(f.expiry).Unix(),
	}
	for k, v := range f.claims {
		claims[k] = v
	}
	if f.nonceOverride != "" {
		claims["nonce"] = f.nonceOverride
	}

	token := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
	token.Header["kid"] = fakeIDPKeyID
	key := f.signWith
	if key == nil {
		key = f.key
	}
	signed, err := token.SignedString(key)
	if err != nil {
		f.t.Fatalf("sign id token: %v", err)
	}
	return signed
}

// setClaims replaces the claim set the next ID token carries.
func (f *fakeIDP) setClaims(c map[string]any) {
	f.claims = c
}

// echoNonce configures the provider to echo the nonce from an authorization
// URL, which is what a conforming provider does.
func (f *fakeIDP) echoNonce(authURL string) {
	f.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		f.t.Fatalf("parse authorization url: %v", err)
	}
	nonce := u.Query().Get("nonce")
	if nonce == "" {
		f.t.Fatal("authorization url carried no nonce")
	}
	if f.claims == nil {
		f.claims = map[string]any{}
	}
	f.claims["nonce"] = nonce
}

// stateFrom extracts the state parameter the service put in the authorization
// URL, standing in for the provider handing it back on the callback.
func stateFrom(t *testing.T, authURL string) string {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse authorization url: %v", err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatal("authorization url carried no state")
	}
	return state
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
