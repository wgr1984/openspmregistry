package authenticator

import (
	"OpenSPMRegistry/config"
	"context"
	"crypto/tls"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"errors"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func Test_AuthCodeURL_GeneratesCorrectURL(t *testing.T) {
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
		} else if r.URL.Path == "/auth" {
			w.Header().Set("Location", `http://`+r.Host+`/callback`)
			w.WriteHeader(http.StatusFound)
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{Issuer: mockServer.URL, ClientId: "client-id", ClientSecret: "client-secret"}}

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	state := "teststate"
	nonce := oauth2.SetAuthURLParam("nonce", "testnonce")
	url := auth.AuthCodeURL(state, nonce)

	expectedPrefix := mockServer.URL + "/auth"
	if !strings.HasPrefix(url, expectedPrefix) {
		t.Errorf("expected URL to start with %s, got %s", expectedPrefix, url)
	}

	if !strings.Contains(url, "state="+state) {
		t.Errorf("expected URL to contain state parameter %s", state)
	}

	if !strings.Contains(url, "nonce=testnonce") {
		t.Errorf("expected URL to contain nonce parameter")
	}
}

func Test_Callback_InvalidState_ReturnsUnauthorized(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"issuer": "http://example.com",
				"authorization_endpoint": "/auth",
				"token_endpoint": "/token",
				"jwks_uri": "/keys"
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	req := httptest.NewRequest("GET", "/callback?state=invalidstate", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "state did not match") {
		t.Errorf("expected response to contain 'state did not match', got %s", w.Body.String())
	}
}

func Test_Callback_ValidStateAndCode_ReturnsToken(t *testing.T) {
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
				"id_token": "id-token"
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	req := httptest.NewRequest("GET", "/callback?state=validstate&code=validcode", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "id-token") {
		t.Errorf("expected response to contain 'id-token', got %s", w.Body.String())
	}
}

func Test_Login_RedirectsToAuthURL(t *testing.T) {
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
		}
	}))
	defer mockServer.Close()

	config := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, config, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected status Found, got %v", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), mockServer.URL) {
		t.Errorf("expected redirect to contain '%s', got %s", mockServer.URL, w.Header().Get("Location"))
	}
}

func Test_Login_RedirectsToAuthURL_NotToSelf(t *testing.T) {
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
		}
	}))
	defer mockServer.Close()

	config := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, config, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	redirectURL := w.Header().Get("Location")
	if strings.Contains(redirectURL, "/login") {
		t.Errorf("redirect URL should not contain /login, got %s", redirectURL)
	}
}

func Test_setCallbackCookie_SetsCorrectCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	setCallbackCookie(w, req, "testname", "testvalue")
	cookie := w.Result().Cookies()[0]

	if cookie.Name != "testname" {
		t.Errorf("expected cookie name 'testname', got %s", cookie.Name)
	}
	if cookie.Value != "testvalue" {
		t.Errorf("expected cookie value 'testvalue', got %s", cookie.Value)
	}
	if cookie.MaxAge != int(time.Hour.Seconds()) {
		t.Errorf("expected cookie MaxAge %d, got %d", int(time.Hour.Seconds()), cookie.MaxAge)
	}
	if cookie.Secure != false {
		t.Errorf("expected cookie Secure false, got %v", cookie.Secure)
	}
	if cookie.HttpOnly != true {
		t.Errorf("expected cookie HttpOnly true, got %v", cookie.HttpOnly)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected cookie SameSite %v, got %v", http.SameSiteStrictMode, cookie.SameSite)
	}
}

func Test_setCallbackCookie_WithTLS_SetsCookieSecure(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com", nil)
	req.TLS = &tls.ConnectionState{} // Simulate TLS connection
	w := httptest.NewRecorder()

	setCallbackCookie(w, req, "testname", "testvalue")
	cookie := w.Result().Cookies()[0]

	if !cookie.Secure {
		t.Errorf("expected cookie Secure true for TLS request, got false")
	}
}

func Test_setCallbackCookie_MultipleCookies(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	setCallbackCookie(w, req, "cookie1", "value1")
	setCallbackCookie(w, req, "cookie2", "value2")

	cookies := w.Result().Cookies()

	if len(cookies) != 2 {
		t.Errorf("expected 2 cookies, got %d", len(cookies))
	}

	cookieMap := make(map[string]string)
	for _, c := range cookies {
		cookieMap[c.Name] = c.Value
	}

	if v, ok := cookieMap["cookie1"]; !ok || v != "value1" {
		t.Errorf("expected cookie1=value1, got %s", v)
	}
	if v, ok := cookieMap["cookie2"]; !ok || v != "value2" {
		t.Errorf("expected cookie2=value2, got %s", v)
	}
}

func Test_setCallbackCookie_InvalidValues(t *testing.T) {
	tests := []struct {
		name      string
		cookieVal string
		wantVal   string
	}{
		{
			name:      "empty value",
			cookieVal: "",
			wantVal:   "",
		},
		{
			name:      "very long value",
			cookieVal: string(make([]byte, 4097)), // Exceeds typical 4KB cookie size limit
			wantVal:   "",                         // Cookie value should be empty when too long
		},
		{
			name:      "invalid characters",
			cookieVal: "value with spaces and ;,/?:@&=+$#",
			wantVal:   "value with spaces and ,/?:@&=+$#", // Semicolon gets stripped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			setCallbackCookie(w, req, "testname", tt.cookieVal)
			cookies := w.Result().Cookies()

			if len(cookies) != 1 {
				t.Errorf("expected 1 cookie, got %d", len(cookies))
			}

			cookie := cookies[0]
			if cookie.Value != tt.wantVal {
				t.Errorf("cookie value was modified, expected %q, got %q", tt.wantVal, cookie.Value)
			}

			// Verify other security attributes remain set
			if !cookie.HttpOnly {
				t.Error("HttpOnly flag should be set")
			}
			if cookie.SameSite != http.SameSiteStrictMode {
				t.Error("SameSite should be strict")
			}
		})
	}
}

func Test_NewOIDCAuthenticatorCodeWithConfig_ProviderError_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mockServer.Close()

	c := config.ServerConfig{
		Auth: config.AuthConfig{
			Issuer:       mockServer.URL, // Use mock server that returns error
			ClientId:     "client-id",
			ClientSecret: "client-secret",
		},
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	if auth != nil {
		t.Errorf("expected nil authenticator when provider creation fails, got %v", auth)
	}
}

func Test_Callback_MissingStateCookie_ReturnsUnauthorized(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/callback?state=validstate&code=validcode", nil)
	// Intentionally not adding state cookie
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when state cookie is missing, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "state not found") {
		t.Errorf("expected error message about missing state, got %s", w.Body.String())
	}
}

func Test_Callback_MissingStateParam_ReturnsUnauthorized(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/callback?code=validcode", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when state parameter is missing, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "state not found") {
		t.Errorf("expected error message about missing state, got %s", w.Body.String())
	}
}

func Test_Callback_MissingCode_ReturnsUnauthorized(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/callback?state=validstate", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when code is missing, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "code not found") {
		t.Errorf("expected error message about missing code, got %s", w.Body.String())
	}
}

func Test_Callback_TokenExchangeError_ReturnsUnauthorized(t *testing.T) {
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
			// Simulate token exchange error
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "invalid_grant"}`))
		}
	}))
	defer mockServer.Close()

	c := config.ServerConfig{Auth: config.AuthConfig{
		Issuer:       mockServer.URL,
		ClientId:     "client-id",
		ClientSecret: "client-secret",
	}}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/callback?state=validstate&code=invalidcode", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when token exchange fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Failed to exchange code for token") {
		t.Errorf("expected error message about token exchange failure, got %s", w.Body.String())
	}
}

func Test_Callback_MissingIdToken_ReturnsUnauthorized(t *testing.T) {
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
			// Return token response without id_token
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/callback?state=validstate&code=validcode", nil)
	req.AddCookie(&http.Cookie{Name: "state", Value: "validstate"})
	w := httptest.NewRecorder()

	auth.Callback(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when id_token is missing, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Failed to get id token") {
		t.Errorf("expected error message about missing id token, got %s", w.Body.String())
	}
}

func Test_Login_WithTLS_SetsCookiesSecure(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "https://example.com/login", nil)
	req.TLS = &tls.ConnectionState{} // Simulate TLS connection
	w := httptest.NewRecorder()

	auth.Login(w, req)

	cookies := w.Result().Cookies()
	for _, cookie := range cookies {
		if !cookie.Secure {
			t.Errorf("expected cookie %s to be secure for TLS request", cookie.Name)
		}
	}
}

func Test_Login_WithExistingAuthHeader_ReturnsEarly(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, temp)

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

func Test_Login_WithNilTemplate_HandlesError(t *testing.T) {
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

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)

	req := httptest.NewRequest("GET", "/login", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK when auth header exists, got %v", w.Code)
	}
	if w.Body.String() != "Bearer test-token" {
		t.Errorf("expected response to contain token from auth header, got %s", w.Body.String())
	}
}

func Test_Login_RandomStateError_ReturnsUnauthorized(t *testing.T) {
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

	mockGenerator := &mockRandomStringGenerator{
		generateFunc: func(length int) (string, error) {
			return "", errors.New("simulated state generation error")
		},
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)
	auth.randomStringGenerator = mockGenerator

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when state generation fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Authentication failed: simulated state generation error") {
		t.Errorf("expected error message about state generation, got %s", w.Body.String())
	}
}

func Test_Login_RandomNonceError_ReturnsUnauthorized(t *testing.T) {
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

	var callCount int
	mockGenerator := &mockRandomStringGenerator{
		generateFunc: func(length int) (string, error) {
			callCount++
			if callCount == 1 {
				// First call (state) succeeds
				return "test-state", nil
			}
			// Second call (nonce) fails
			return "", errors.New("simulated nonce generation error")
		},
	}

	auth := NewOIDCAuthenticatorCodeWithConfig(ctx, c, &oidc.Config{
		ClientID:                   "client-id",
		InsecureSkipSignatureCheck: true,
	}, nil)
	auth.randomStringGenerator = mockGenerator

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()

	auth.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized when nonce generation fails, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Authentication failed: simulated nonce generation error") {
		t.Errorf("expected error message about nonce generation, got %s", w.Body.String())
	}
}

// Mock types and implementations

type mockRandomStringGenerator struct {
	generateFunc func(length int) (string, error)
}

func (m *mockRandomStringGenerator) RandomString(length int) (string, error) {
	return m.generateFunc(length)
}
