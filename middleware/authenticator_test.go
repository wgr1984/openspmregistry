package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func Test_NewAuthentication_TokenAuthenticator_RegistersCallbackHandler(t *testing.T) {
	router := http.NewServeMux()
	auth := &MockTokenAuthenticator{}
	NewAuthentication(auth, router)

	req := httptest.NewRequest("GET", "/callback", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// check if callback function was called
	if count, ok := auth.methodCallCount["Callback"]; ok && count == 0 {
		t.Errorf("expected Callback to be called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", w.Code)
	}
}

func Test_NewAuthentication_OidcAuthenticator_RegistersLoginHandler(t *testing.T) {
	router := http.NewServeMux()
	auth := &MockOidcAuthenticator{}
	NewAuthentication(auth, router)

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if count, ok := auth.methodCallCount["Login"]; !ok || count == 0 {
		t.Errorf("expected Login to be called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("expected status OK, got %v", w.Code)
	}
}

func Test_HandleFunc_UnauthorizedRequest_Returns401(t *testing.T) {
	router := http.NewServeMux()
	auth := &MockAuthenticator{shouldAuthenticate: false}
	a := NewAuthentication(auth, router)

	a.HandleFunc("/protected", func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)

	if count, ok := auth.methodCallCount["Authenticate"]; !ok || count == 0 {
		t.Errorf("expected Authenticate to be called")
	}

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status Unauthorized, got %v", w.Code)
	}
}

func Test_HandleFunc_AuthorizedRequest_CallsNextHandler(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	router := http.NewServeMux()
	auth := &MockAuthenticator{shouldAuthenticate: true}
	a := NewAuthentication(auth, router)

	called := false
	a.HandleFunc("/protected", func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)

	if count, ok := auth.methodCallCount["Authenticate"]; !ok || count == 0 {
		t.Errorf("expected Authenticate to be called")
	}

	if !called {
		t.Errorf("expected handler to be called")
	}
}

type MockTokenAuthenticator struct {
	methodCallCount map[string]int
}

func (m *MockTokenAuthenticator) increaseCallCount(method string) {
	if m.methodCallCount == nil {
		m.methodCallCount = make(map[string]int)
	}
	m.methodCallCount[method]++
}

func (m *MockTokenAuthenticator) AuthCodeURL(state string, nonce oauth2.AuthCodeOption) string {
	m.increaseCallCount("AuthCodeURL")
	return "https://example.com"
}

func (m *MockTokenAuthenticator) Login(w http.ResponseWriter, r *http.Request) {
	m.increaseCallCount("Login")
	w.WriteHeader(http.StatusOK)
}

func (m *MockTokenAuthenticator) CheckAuthHeaderPresent(w http.ResponseWriter, r *http.Request) bool {
	m.increaseCallCount("CheckAuthHeaderPresent")
	return true
}

func (m *MockTokenAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	m.increaseCallCount("Authenticate")
	return "", nil
}

func (m *MockTokenAuthenticator) Callback(w http.ResponseWriter, r *http.Request) {
	m.increaseCallCount("Callback")
	w.WriteHeader(http.StatusOK)
}

type MockOidcAuthenticator struct {
	methodCallCount map[string]int
}

func (m *MockOidcAuthenticator) increaseCallCount(method string) {
	if m.methodCallCount == nil {
		m.methodCallCount = make(map[string]int)
	}
	m.methodCallCount[method]++
}

func (m *MockOidcAuthenticator) CheckAuthHeaderPresent(w http.ResponseWriter, r *http.Request) bool {
	m.increaseCallCount("CheckAuthHeaderPresent")
	return true
}

func (m *MockOidcAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	m.increaseCallCount("Authenticate")
	return "", nil
}

func (m *MockOidcAuthenticator) Login(w http.ResponseWriter, r *http.Request) {
	m.increaseCallCount("Login")
	w.WriteHeader(http.StatusOK)
}

type MockAuthenticator struct {
	shouldAuthenticate bool
	methodCallCount    map[string]int
}

func (m *MockAuthenticator) increaseCallCount(method string) {
	if m.methodCallCount == nil {
		m.methodCallCount = make(map[string]int)
	}
	m.methodCallCount[method]++
}

func (m *MockAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (string, error) {
	m.increaseCallCount("Authenticate")
	if !m.shouldAuthenticate {
		return "", fmt.Errorf("unauthorized")
	}
	return "", nil
}
