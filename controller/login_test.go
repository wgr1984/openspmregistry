package controller

import (
	"OpenSPMRegistry/config"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_LoginAction_Success(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("POST", "/login", nil)
	w := httptest.NewRecorder()

	c.LoginAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

func Test_LoginAction_LogsDebugInfo(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("POST", "/login", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()

	c.LoginAction(w, req)

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "LoginAction Request") {
		t.Errorf("expected log message containing 'LoginAction Request', got %q", logOutput)
	}
	if !strings.Contains(logOutput, "Bearer ****") {
		t.Errorf("expected masked Bearer token in log, got %q", logOutput)
	}
}

func Test_LoginAction_LogsDebugInfo_BasicAuth(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("POST", "/login", nil)
	req.Header.Set("Authorization", "Basic test-credentials")
	w := httptest.NewRecorder()

	c.LoginAction(w, req)

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "LoginAction Request") {
		t.Errorf("expected log message containing 'LoginAction Request', got %q", logOutput)
	}
	if !strings.Contains(logOutput, "Basic ****") {
		t.Errorf("expected masked Basic auth in log, got %q", logOutput)
	}
}
