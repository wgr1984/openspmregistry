package authenticator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_Authenticate_NoOpAuthenticator_ReturnsNilError(t *testing.T) {
	auth := &NoOpAuthenticator{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if token != "noop" {
		t.Errorf("expected 'noop' token, got %v", token)
	}
}

func Test_Authenticate_NoOpAuthenticator_ReturnsNoopToken(t *testing.T) {
	auth := &NoOpAuthenticator{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if token != "noop" {
		t.Errorf("expected 'noop' token, got %s", token)
	}
}

func Test_Callback_NoOpAuthenticator_ReturnsUnauthorized(t *testing.T) {
	auth := &NoOpAuthenticator{}
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	auth.Callback(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized, got %v", w.Code)
	}
	if !strings.Contains(w.Body.String(), "callback not supported") {
		t.Errorf("expected response to contain 'callback not supported', got %s", w.Body.String())
	}
}
