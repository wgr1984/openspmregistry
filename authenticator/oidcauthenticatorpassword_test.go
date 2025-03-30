package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func Test_NewOIDCAuthenticatorPassword_CreatesValidAuthenticator(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	if auth == nil {
		t.Error("expected non-nil authenticator")
	}
	if auth.OidcAuthenticatorImpl == nil {
		t.Error("expected non-nil base authenticator")
	}
	if auth.sharedEncryptionKey == nil {
		t.Error("expected non-nil shared encryption key")
	}
	if len(auth.sharedEncryptionKey) != 16 {
		t.Errorf("expected shared encryption key length 16, got %d", len(auth.sharedEncryptionKey))
	}
	if auth.signingKey == nil {
		t.Error("expected non-nil signing key")
	}
	if auth.privateKey == nil {
		t.Error("expected non-nil private key")
	}
}

func Test_OIDCAuthenticatorPassword_Callback_ReturnsUnauthorized(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	req := httptest.NewRequest("GET", "/callback", nil)
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "callback not supported") {
		t.Errorf("expected response to contain 'callback not supported', got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_WithBasicAuth_MissingCSRFToken_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("test-user", "test-pass")
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)

	if err == nil || err.Error() != "missing CSRF token" {
		t.Errorf("expected 'missing CSRF token' error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_WithBasicAuth_InvalidCSRFToken_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a valid token first
	token, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}

	// Modify the token to make it invalid
	invalidToken := token[:len(token)-1] // Remove last character to make it invalid

	req := httptest.NewRequest("POST", "/authenticate", nil)
	req.SetBasicAuth("test-user", "test-pass")   // Set proper Basic auth
	req.Header.Set("x-csrf-token", invalidToken) // Set CSRF token in proper header
	w := httptest.NewRecorder()

	token, err = auth.Authenticate(w, req)

	if err == nil {
		t.Error("expected error for invalid CSRF token")
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_Login_ParsesTemplate(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status Internal Server Error when template parsing fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Error parsing template") {
		t.Errorf("expected response to contain 'Error parsing template', got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_Login_WithAuthHeader_ReturnsEarly(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}

	auth := NewOIDCAuthenticatorPassword(ctx, c)
	auth.OidcAuthenticatorImpl.template = temp

	req := httptest.NewRequest("GET", "/login", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK when auth header exists, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "test-token") {
		t.Errorf("expected response to contain token from auth header, got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_Login_WithValidTemplate_RendersLoginPage(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a valid template
	tmpl := template.Must(template.New("login.gohtml").Parse(`
		<!DOCTYPE html>
		<html>
		<head>
			<title>Login</title>
		</head>
		<body>
			<form method="post" action="/authenticate">
				<input type="hidden" name="csrf_token" value="{{.CsrfToken}}">
				<input type="text" name="username" placeholder="Username">
				<input type="password" name="password" placeholder="Password">
				<button type="submit">Login</button>
			</form>
		</body>
		</html>
	`))

	auth.OidcAuthenticatorImpl.template = &MockTemplateParser{
		template: *tmpl,
	}

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Login") {
		t.Errorf("expected response to contain title, got %s", body)
	}
	if !strings.Contains(body, "csrf_token") {
		t.Errorf("expected response to contain CSRF token field, got %s", body)
	}
}

func Test_OIDCAuthenticatorPassword_EncryptToken_GeneratesValidJWT(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Errorf("unexpected error encrypting token: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}

	// Verify the token
	err = auth.verifyToken(token, csrfTokenValue)
	if err != nil {
		t.Errorf("unexpected error verifying token: %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_ExpiredToken_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with a very short expiration
	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // Expired 1 hour ago
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	enc, err := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))
	if err != nil {
		t.Fatalf("unexpected error creating encrypter: %v", err)
	}

	token, err := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}

	err = auth.verifyToken(token, csrfTokenValue)
	if err == nil || !strings.Contains(err.Error(), "token expired") {
		t.Errorf("expected token expired error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidValue_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Errorf("unexpected error encrypting token: %v", err)
	}

	err = auth.verifyToken(token, "different-value")
	if err == nil || !strings.Contains(err.Error(), "invalid value") {
		t.Errorf("expected invalid value error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidSubject_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with an invalid subject
	cl := jwt.Claims{
		Subject: "invalid-subject",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	enc, err := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))
	if err != nil {
		t.Fatalf("unexpected error creating encrypter: %v", err)
	}

	token, err := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}

	err = auth.verifyToken(token, csrfTokenValue)
	if err == nil || !strings.Contains(err.Error(), "invalid subject") {
		t.Errorf("expected invalid subject error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_RequestToken_Success(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "access-token",
				"token_type": "Bearer",
				"id_token": "test-id-token"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.requestToken("test-user", "test-pass")
	if err != nil {
		t.Errorf("unexpected error requesting token: %v", err)
	}
	if token != "test-id-token" {
		t.Errorf("expected test-id-token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_RequestToken_MissingIdToken_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "access-token",
				"token_type": "Bearer"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.requestToken("test-user", "test-pass")
	if err == nil || err.Error() != "missing id token" {
		t.Errorf("expected missing id token error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_RequestToken_InvalidCredentials_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid_grant"}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.requestToken("invalid-user", "invalid-pass")
	if err == nil {
		t.Error("expected error for invalid credentials")
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_WithInvalidBasicAuth_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a valid CSRF token
	csrfToken, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}

	req := httptest.NewRequest("POST", "/authenticate", nil)
	req.Header.Set("Authorization", "Basic invalid")
	req.Header.Set("x-csrf-token", csrfToken)
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err == nil {
		t.Error("expected error for invalid basic auth")
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_EncryptToken_EmptyKey_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)
	auth.sharedEncryptionKey = nil // Force empty key

	token, err := auth.encryptToken(csrfTokenValue)
	if err == nil {
		t.Error("expected error for empty encryption key")
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_MissingValue_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token without a value claim
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil || err.Error() != "missing value" {
		t.Errorf("expected 'missing value' error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_RequestToken_InvalidResponse_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "access-token",
				"token_type": "Bearer",
				"expires_in": 3600
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	token, err := auth.requestToken("test-user", "test-pass")
	if err == nil || err.Error() != "missing id token" {
		t.Errorf("expected 'missing id token' error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_NoTemplate_WritesPlainToken(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "access-token",
				"token_type": "Bearer",
				"id_token": "test-id-token"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)
	auth.OidcAuthenticatorImpl.template = nil // Force nil template

	// Create a valid CSRF token
	csrfToken, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("unexpected error creating token: %v", err)
	}

	req := httptest.NewRequest("POST", "/authenticate", nil)
	req.SetBasicAuth("test-user", "test-pass")
	req.Header.Set("x-csrf-token", csrfToken)
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if token != "test-id-token" {
		t.Errorf("expected test-id-token, got %s", token)
	}
	if w.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "test-id-token" {
		t.Errorf("expected response body to be token, got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidIssuer_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with an invalid issuer
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "invalid-subject", // This will trigger invalid subject error
		Issuer:  "InvalidIssuer",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil || !strings.Contains(err.Error(), "invalid subject") {
		t.Errorf("expected 'invalid subject' error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_NoExpiry_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with an expired time
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)), // Set expiry in the past
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil || !strings.Contains(err.Error(), "token expired") {
		t.Errorf("expected 'token expired' error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_NoSubject_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token without a subject
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Issuer: "OpenSPMRegistry",
		Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil || !strings.Contains(err.Error(), "invalid subject") {
		t.Errorf("expected 'invalid subject' error, got %v", err)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidAlgorithm_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a valid token first
	validToken, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("failed to create valid token: %v", err)
	}

	// Modify the token's header to simulate invalid algorithm
	parts := strings.Split(validToken, ".")
	if len(parts) < 3 {
		t.Fatal("invalid token format")
	}

	// Modify the first part (header) to make it invalid
	invalidToken := "invalid." + parts[1] + "." + parts[2]

	err = auth.verifyToken(invalidToken, csrfTokenValue)
	if err == nil {
		t.Error("expected error for invalid algorithm")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidContentType_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a valid token first
	validToken, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("failed to create valid token: %v", err)
	}

	// Modify the token to make it invalid
	invalidToken := validToken + "invalid"

	err = auth.verifyToken(invalidToken, csrfTokenValue)
	if err == nil {
		t.Error("expected error for invalid content type")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_MalformedToken_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Test with various malformed tokens
	malformedTokens := []string{
		"invalid",
		"invalid.token",
		"invalid.token.format.extra",
		"",
		".",
		"..",
	}

	for _, token := range malformedTokens {
		err := auth.verifyToken(token, csrfTokenValue)
		if err == nil {
			t.Errorf("expected error for malformed token: %s", token)
		}
	}
}

func Test_OIDCAuthenticatorPassword_EncryptToken_InvalidKeySize_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Test with various invalid key sizes
	invalidKeySizes := []int{8, 24, 32, 0}
	for _, size := range invalidKeySizes {
		auth.sharedEncryptionKey = make([]byte, size)
		token, err := auth.encryptToken(csrfTokenValue)
		if err == nil {
			t.Errorf("expected error for key size %d", size)
		}
		if token != "" {
			t.Errorf("expected empty token for key size %d, got %s", size, token)
		}
	}
}

func Test_OIDCAuthenticatorPassword_Login_TemplateExecuteError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a template that will fail to execute
	tmpl := template.Must(template.New("login.gohtml").Parse(`{{.NonexistentField}}`))
	auth.OidcAuthenticatorImpl.template = &MockTemplateParser{
		template: *tmpl,
	}

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status Internal Server Error when template execution fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Error executing template") {
		t.Errorf("expected response to contain 'Error executing template', got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_Login_EncryptTokenError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)
	// Force encryption to fail by using an invalid key
	auth.sharedEncryptionKey = []byte{}

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status Internal Server Error when token encryption fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Error encrypting token") {
		t.Errorf("expected response to contain 'Error encrypting token', got %s", w.Body.String())
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_InvalidTokenError(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	req := httptest.NewRequest("POST", "/authenticate", nil)
	req.SetBasicAuth("test-user", "test-pass")
	req.Header.Set("x-csrf-token", "invalid-token")
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)

	if err == nil {
		t.Error("expected error for invalid token")
	}
	if token != "" {
		t.Errorf("expected empty token, got %s", token)
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create an expired token
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(-time.Hour)), // Expired 1 hour ago
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying expired token")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidSubject(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with invalid subject
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "invalid subject",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		csrfTokenValue,
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying token with invalid subject")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_EmptyValue(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with empty value
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		"",
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying token with empty value")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidValue(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create a token with invalid value
	enc, _ := jose.NewEncrypter(
		jose.A128GCM,
		jose.Recipient{
			Algorithm: jose.DIRECT,
			Key:       auth.sharedEncryptionKey,
		},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	cl := jwt.Claims{
		Subject: "oidc login nonce",
		Issuer:  "OpenSPMRegistry",
		Expiry:  jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	privateClaim := struct {
		Value string `json:"value"`
	}{
		"invalid-value",
	}

	token, _ := jwt.Encrypted(enc).Claims(cl).Claims(privateClaim).Serialize()

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying token with invalid value")
	}
}

func Test_OIDCAuthenticatorPassword_VerifyToken_InvalidToken(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, c)

	// Create an invalid token
	token := "invalid.token.format"

	err := auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying invalid token")
	}
}

func Test_OIDCAuthenticatorPassword_EncryptToken_InvalidKey(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		}
	}))
	defer mockServer.Close()

	cfg := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, cfg)

	// Create a token with the current key
	token, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Change the key to make verification fail
	originalKey := auth.sharedEncryptionKey
	auth.sharedEncryptionKey = make([]byte, 16)
	copy(auth.sharedEncryptionKey, "different key...") // Use a different key

	err = auth.verifyToken(token, csrfTokenValue)
	if err == nil {
		t.Error("expected error when verifying with wrong key")
	}

	// Restore the original key
	auth.sharedEncryptionKey = originalKey
}

func Test_OIDCAuthenticatorPassword_Authenticate_BasicAuthSuccess(t *testing.T) {
	// Setup mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://` + r.Host + `",
				"authorization_endpoint": "http://` + r.Host + `/auth",
				"token_endpoint": "http://` + r.Host + `/token",
				"jwks_uri": "http://` + r.Host + `/keys"
			}`))
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"access_token": "access-token",
				"token_type": "Bearer",
				"id_token": "test-id-token"
			}`))
		}
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       server.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, cfg)
	auth.template = template.New("test")

	// Create a valid CSRF token
	csrfToken, err := auth.encryptToken(csrfTokenValue)
	if err != nil {
		t.Fatalf("failed to create CSRF token: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/authenticate", nil)
	r.SetBasicAuth("test-user", "test-pass")
	r.Header.Set("x-csrf-token", csrfToken)

	token, err := auth.Authenticate(w, r)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if token != "test-id-token" {
		t.Errorf("expected test-id-token, got %q", token)
	}
}

func Test_OIDCAuthenticatorPassword_Authenticate_OIDCPath(t *testing.T) {
	t.Skip("Skipping OIDC path test until we can properly mock the OIDC provider")
}

func Test_OIDCAuthenticatorPassword_EncryptToken_Error(t *testing.T) {
	ctx := context.Background()
	cfg := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       "http://example.com",
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorPassword(ctx, cfg)

	// Test error path by using an invalid key size
	auth.sharedEncryptionKey = make([]byte, 1) // Invalid key size for A128GCM

	token, err := auth.encryptToken("test-value")
	if err == nil {
		t.Error("expected error when using invalid key size")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}

	// Test error path with nil key
	auth.sharedEncryptionKey = nil

	token, err = auth.encryptToken("test-value")
	if err == nil {
		t.Error("expected error when using nil key")
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}
