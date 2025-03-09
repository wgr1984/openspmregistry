package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_OIDC_Authenticate_NoAuthorizationHeader_ReturnsError(t *testing.T) {
	ctx := context.Background()
	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: "http://example.com", ClientId: "client-id", ClientSecret: "client-secret"}}
	auth := NewOIDCAuthenticator(ctx, c)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	err, _ := auth.Authenticate(w, req)
	if err == nil || err.Error() != "authorization header not found" {
		t.Errorf("expected 'authorization header not found' error, got %v", err)
	}
}

func Test_OIDC_Authenticate_InvalidBearerToken_ReturnsError(t *testing.T) {
	ctx := context.Background()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"issuer": "http://` + r.Host + `",
					"jwks_uri": "http://` + r.Host + `/keys"
				}`))
		} else if r.URL.Path == "/keys" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"keys": [
						{
							"kty": "RSA",
							"kid": "test-key",
							"use": "sig",
							"alg": "RS256",
							"n": "test-n",
							"e": "AQAB"
						}
					]
				}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}

	auth := NewOIDCAuthenticatorWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, template.Must(template.New("test").Parse("{{.Token}}")))

	iss := "http://" + mockServer.Listener.Addr().String()
	sub := "test-user"
	aud := "client-id"
	exp := time.Now().Add(-1 * time.Hour)

	base64Token, err := createJWT(iss, sub, aud, exp)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+base64Token)
	w := httptest.NewRecorder()

	err, _ = auth.Authenticate(w, req)
	if err == nil || !strings.Contains(err.Error(), "oidc: token is expired") {
		t.Errorf("expected 'oidc: ' error, got %v", err)
	}
}

func Test_OIDC_Authenticate_ValidBearerToken_ReturnsNil(t *testing.T) {
	ctx := context.Background()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"issuer": "http://` + r.Host + `",
					"jwks_uri": "http://` + r.Host + `/keys"
				}`))
		} else if r.URL.Path == "/keys" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"keys": [
						{
							"kty": "RSA",
							"kid": "test-key",
							"use": "sig",
							"alg": "RS256",
							"n": "test-n",
							"e": "AQAB"
						}
					]
				}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}
	auth := NewOIDCAuthenticatorWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, template.Must(template.New("test").Parse("{{.Token}}")))

	iss := "http://" + mockServer.Listener.Addr().String()
	sub := "test-user"
	aud := "client-id"
	exp := time.Now().Add(+1 * time.Hour)

	jwtToken, err := createJWT(iss, sub, aud, exp)
	if err != nil {
		t.Errorf("error creating JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()

	err, token := auth.Authenticate(w, req)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if token != jwtToken {
		t.Errorf("expected 'valid-token', got %s", token)
	}
}

func Test_OIDC_Authenticate_AutherntorButNoBearerToken_ReturnsError(t *testing.T) {
	ctx := context.Background()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"issuer": "http://` + r.Host + `",
					"jwks_uri": "http://` + r.Host + `/keys"
				}`))
		} else if r.URL.Path == "/keys" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"keys": [
						{
							"kty": "RSA",
							"kid": "test-key",
							"use": "sig",
							"alg": "RS256",
							"n": "test-n",
							"e": "AQAB"
						}
					]
				}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}
	auth := NewOIDCAuthenticatorWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, template.Must(template.New("test").Parse("{{.Token}}")))

	iss := "http://" + mockServer.Listener.Addr().String()
	sub := "test-user"
	aud := "client-id"
	exp := time.Now().Add(+1 * time.Hour)

	jwtToken, err := createJWT(iss, sub, aud, exp)
	if err != nil {
		t.Errorf("error creating JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", jwtToken)
	w := httptest.NewRecorder()

	err, _ = auth.Authenticate(w, req)
	if err == nil || err.Error() != "invalid authorization header" {
		t.Errorf("expected invalid authorization header, got nil")
	}
}

func Test_OIDC_CheckAuthHeaderPresent_WithAuthHeader_WritesTokenToResponse(t *testing.T) {
	ctx := context.Background()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"issuer": "http://` + r.Host + `",
					"jwks_uri": "http://` + r.Host + `/keys"
				}`))
		} else if r.URL.Path == "/keys" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"keys": [
						{
							"kty": "RSA",
							"kid": "test-key",
							"use": "sig",
							"alg": "RS256",
							"n": "test-n",
							"e": "AQAB"
						}
					]
				}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}

	auth := NewOIDCAuthenticatorWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	iss := "http://" + mockServer.Listener.Addr().String()
	sub := "test-user"
	aud := "client-id"
	exp := time.Now().Add(+1 * time.Hour)

	jwtToken, err := createJWT(iss, sub, aud, exp)
	if err != nil {
		t.Errorf("error creating JWT: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	w := httptest.NewRecorder()

	result := auth.CheckAuthHeaderPresent(w, req)
	if !result {
		t.Errorf("expected true, got false")
	}
	if !strings.Contains(w.Body.String(), "Bearer "+jwtToken) {
		t.Errorf("expected response to contain 'Bearer valid-token', got %s", w.Body.String())
	}
}

func Test_OIDC_CheckAuthHeaderPresent_WithoutAuthHeader_ReturnsFalse(t *testing.T) {
	ctx := context.Background()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"issuer": "http://` + r.Host + `",
					"jwks_uri": "http://` + r.Host + `/keys"
				}`))
		} else if r.URL.Path == "/keys" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
					"keys": [
						{
							"kty": "RSA",
							"kid": "test-key",
							"use": "sig",
							"alg": "RS256",
							"n": "test-n",
							"e": "AQAB"
						}
					]
				}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}
	auth := NewOIDCAuthenticatorWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	result := auth.CheckAuthHeaderPresent(w, req)
	if result {
		t.Errorf("expected false, got true")
	}
	if w.Body.String() != "" {
		t.Errorf("expected empty response body, got %s", w.Body.String())
	}
}

func createJWT(iss string, sub string, aud string, exp time.Time) (string, error) {
	// Add claims to the JWT
	claims := jwt.Claims{
		Issuer:   iss,
		Subject:  sub,
		Expiry:   jwt.NewNumericDate(exp),
		Audience: jwt.Audience{aud},
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	signingKey := jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}
	signer, err := jose.NewSigner(signingKey, nil)
	if err != nil {
		return "", err
	}

	token, err := jwt.Signed(signer).Claims(claims).Serialize()

	return token, nil
}
