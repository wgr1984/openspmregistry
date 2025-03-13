package controller

import (
	"OpenSPMRegistry/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func Test_MainAction_Returns404(t *testing.T) {
	// Create controller with minimal config
	c := NewController(config.ServerConfig{}, nil)

	// Create test request and response recorder
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Call MainAction
	c.MainAction(w, req)

	// Check status code
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, w.Code)
	}

	// Check response headers
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/problem+json" {
		t.Errorf("expected Content-Type %s, got %s", "application/problem+json", contentType)
	}

	contentLanguage := w.Header().Get("Content-Language")
	if contentLanguage != "en" {
		t.Errorf("expected Content-Language %s, got %s", "en", contentLanguage)
	}

	// Check response body
	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "Not found" {
		t.Errorf("expected error detail %q, got %q", "Not found", response.Detail)
	}
}

func Test_StaticAction_ServesFiles(t *testing.T) {
	// Create a temporary directory for static files
	tmpDir, err := os.MkdirTemp("", "static")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test static file
	testContent := "test static content"
	testFilePath := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create controller with minimal config
	c := NewController(config.ServerConfig{}, nil)

	// Create test request and response recorder
	req := httptest.NewRequest("GET", "/test.txt", nil)
	w := httptest.NewRecorder()

	// Create symbolic link from static directory to temp directory
	if err := os.MkdirAll("static", 0755); err != nil {
		t.Fatalf("failed to create static directory: %v", err)
	}
	defer os.RemoveAll("static")

	if err := os.Symlink(testFilePath, "static/test.txt"); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Call StaticAction
	c.StaticAction(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check response body
	if w.Body.String() != testContent {
		t.Errorf("expected body %q, got %q", testContent, w.Body.String())
	}
}

func Test_StaticAction_Returns404_ForNonExistentFile(t *testing.T) {
	// Create controller with minimal config
	c := NewController(config.ServerConfig{}, nil)

	// Create test request and response recorder
	req := httptest.NewRequest("GET", "/nonexistent.txt", nil)
	w := httptest.NewRecorder()

	// Call StaticAction
	c.StaticAction(w, req)

	// Check status code
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, w.Code)
	}
}

func Test_NewController_InitializesCorrectly(t *testing.T) {
	// Create test config and repo
	cfg := config.ServerConfig{
		Hostname: "test-host",
		Port:     8080,
	}
	mockRepo := &MockRepo{}

	// Create controller
	c := NewController(cfg, mockRepo)

	// Check controller fields
	if c.config.Hostname != cfg.Hostname {
		t.Errorf("expected config hostname %s, got %s", cfg.Hostname, c.config.Hostname)
	}
	if c.config.Port != cfg.Port {
		t.Errorf("expected config port %d, got %d", cfg.Port, c.config.Port)
	}
	if c.repo != mockRepo {
		t.Error("repo not set correctly")
	}
}
