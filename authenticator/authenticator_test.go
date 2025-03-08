package authenticator

import (
	"OpenSPMRegistry/config"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_WriteTokenOutput_ValidToken_WritesTokenToResponse(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token"

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token}}")),
	}
	writeTokenOutput(w, token, temp)

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), token) {
		t.Errorf("expected response to contain token, got %s", w.Body.String())
	}
}

func Test_WriteTokenOutput_TemplateParseError_ReturnsInternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token"

	temp := &MockTemplateParserError{}
	writeTokenOutput(w, token, temp)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status InternalServerError, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Error parsing template") {
		t.Errorf("expected response to contain error message, got %s", w.Body.String())
	}
}

func Test_WriteTokenOutput_TemplateExecuteError_ReturnsInternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	token := "test-token"

	temp := &MockTemplateParser{
		template: *template.Must(template.New("test").Parse("{{.Token2}}")),
	}
	writeTokenOutput(w, token, temp)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status InternalServerError, got %v", w.Code)
	}
}

func Test_CreateAuthenticator_AuthDisabled_ReturnsNoOpAuthenticator(t *testing.T) {
	c := config.ServerConfig{Auth: config.AuthConfig{Enabled: false}}
	auth := CreateAuthenticator(c)

	if _, ok := auth.(*NoOpAuthenticator); !ok {
		t.Errorf("expected NoOpAuthenticator, got %T", auth)
	}
}

func Test_CreateAuthenticator_OidcCode_ReturnsOIDCAuthenticatorCode(t *testing.T) {
	c := config.ServerConfig{Auth: config.AuthConfig{Enabled: true, Type: "oidc", GrantType: "code"}}
	auth := CreateAuthenticator(c)

	if _, ok := auth.(interface{}).(OidcAuthenticatorCode); !ok {
		t.Errorf("expected OIDCAuthenticatorCode, got %T", auth)
	}
}

func Test_CreateAuthenticator_OidcPassword_ReturnsOIDCAuthenticatorPassword(t *testing.T) {
	c := config.ServerConfig{Auth: config.AuthConfig{Enabled: true, Type: "oidc", GrantType: "password"}}
	auth := CreateAuthenticator(c)

	if _, ok := auth.(interface{}).(OidcAuthenticatorPassword); !ok {
		t.Errorf("expected OIDCAuthenticatorPassword, got %T", auth)
	}
}

func Test_CreateAuthenticator_Basic_ReturnsBasicAuthenticator(t *testing.T) {
	c := config.ServerConfig{Auth: config.AuthConfig{Enabled: true, Type: "basic", Users: []config.User{
		{Username: "test", Password: "test"},
	}}}
	auth := CreateAuthenticator(c)

	if _, ok := auth.(*BasicAuthenticator); !ok {
		t.Errorf("expected BasicAuthenticator, got %T", auth)
	}
}

func Test_CreateAuthenticator_UnknownType_ReturnsNoOpAuthenticator(t *testing.T) {
	c := config.ServerConfig{Auth: config.AuthConfig{Enabled: true, Type: "unknown"}}
	auth := CreateAuthenticator(c)

	if _, ok := auth.(*NoOpAuthenticator); !ok {
		t.Errorf("expected NoOpAuthenticator, got %T", auth)
	}
}

type MockTemplateParser struct {
	template template.Template
}

func (m MockTemplateParser) ParseFiles(filenames ...string) (*template.Template, error) {
	return &m.template, nil
}

type MockTemplateParserError struct {
	template template.Template
}

func (m MockTemplateParserError) ParseFiles(filenames ...string) (*template.Template, error) {
	return nil, errors.New("Error parsing template")
}
