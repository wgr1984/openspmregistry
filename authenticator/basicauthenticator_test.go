package authenticator

import (
	"OpenSPMRegistry/config"
	"log/slog"
	"net/http/httptest"
	"testing"
)

func Test_Authenticate_NoAuthorizationHeader_ReturnsError(t *testing.T) {
	users := []config.User{{Username: "user", Password: hashPassword("pass")}}
	auth := NewBasicAuthenticator(users)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err == nil || err.Error() != "authorization header not found" {
		t.Errorf("expected 'authorization header not found' error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %v", token)
	}
}

func Test_Authenticate_MissingCredentials_ReturnsError(t *testing.T) {
	users := []config.User{{Username: "user", Password: hashPassword("pass")}}
	auth := NewBasicAuthenticator(users)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic ")
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err == nil || err.Error() != "missing credentials" {
		t.Errorf("expected 'missing credentials' error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %v", token)
	}
}

func Test_Authenticate_InvalidUsernameOrPassword_ReturnsError(t *testing.T) {
	users := []config.User{{Username: "user", Password: hashPassword("pass")}}
	auth := NewBasicAuthenticator(users)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user", "wrongpass")
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err == nil || err.Error() != "invalid username or password" {
		t.Errorf("expected 'invalid username or password' error, got %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %v", token)
	}
}

func Test_Authenticate_ValidCredentials_ReturnsNil(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	users := []config.User{{Username: "user", Password: hashPassword("pass")}}
	auth := NewBasicAuthenticator(users)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user", "pass")
	w := httptest.NewRecorder()

	token, err := auth.Authenticate(w, req)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if token != hashPassword("pass") {
		t.Errorf("expected hashed password, got %s", token)
	}
}

func Test_HashPassword_ConsistentHashing(t *testing.T) {
	password := "testpassword"
	hashed1 := hashPassword(password)
	hashed2 := hashPassword(password)

	if hashed1 != hashed2 {
		t.Errorf("expected consistent hashing, got %s and %s", hashed1, hashed2)
	}
}

func Test_HashPassword_DifferentPasswords(t *testing.T) {
	password1 := "testpassword1"
	password2 := "testpassword2"
	hashed1 := hashPassword(password1)
	hashed2 := hashPassword(password2)

	if hashed1 == hashed2 {
		t.Errorf("expected different hashed passwords, got same")
	}
}
