package controller

import (
	"OpenSPMRegistry/config"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_LookupAction_MissingAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "wrong accept header" {
		t.Errorf("expected error detail %q, got %q", "wrong accept header", response.Detail)
	}
}

func Test_LookupAction_InvalidAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift") // Should be json, not swift

	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status code %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "unsupported media type: swift" {
		t.Errorf("expected error detail %q, got %q", "unsupported media type: swift", response.Detail)
	}
}

func Test_LookupAction_MissingURL_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "url is required" {
		t.Errorf("expected error detail %q, got %q", "url is required", response.Detail)
	}
}

func Test_LookupAction_URLNotFound_ReturnsNotFound(t *testing.T) {
	mockRepo := &MockLookupRepo{identifiers: nil} // Use our MockLookupRepo with nil identifiers
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/lookup?url=nonexistent", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "nonexistent not found" {
		t.Errorf("expected error detail %q, got %q", "nonexistent not found", response.Detail)
	}
}

func Test_LookupAction_Success(t *testing.T) {
	expectedIdentifiers := []string{"id1", "id2"}
	mockRepo := &MockLookupRepo{identifiers: expectedIdentifiers}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check headers
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type %s, got %s", "application/json", contentType)
	}

	contentVersion := w.Header().Get("Content-Version")
	if contentVersion != "1" {
		t.Errorf("expected Content-Version %s, got %s", "1", contentVersion)
	}

	// Check response body
	var response struct {
		Identifiers []string `json:"identifiers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if len(response.Identifiers) != len(expectedIdentifiers) {
		t.Errorf("expected %d identifiers, got %d", len(expectedIdentifiers), len(response.Identifiers))
	}
	for i, id := range expectedIdentifiers {
		if response.Identifiers[i] != id {
			t.Errorf("expected identifier %q at position %d, got %q", id, i, response.Identifiers[i])
		}
	}
}

func Test_LookupAction_InvalidAPIVersion_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v2+json") // Invalid version number

	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "unsupported API version: 2" {
		t.Errorf("expected error detail %q, got %q", "unsupported API version: 2", response.Detail)
	}
}

func Test_LookupAction_EmptyURL_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup?url=", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "url is required" {
		t.Errorf("expected error detail %q, got %q", "url is required", response.Detail)
	}
}

func Test_LookupAction_MultipleAcceptHeaders_UsesFirst(t *testing.T) {
	expectedIdentifiers := []string{"id1"}
	mockRepo := &MockLookupRepo{identifiers: expectedIdentifiers}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Add("Accept", "application/vnd.swift.registry.v1+json")
	req.Header.Add("Accept", "application/vnd.swift.registry.v1+swift") // Second header should be ignored
	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response struct {
		Identifiers []string `json:"identifiers"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if len(response.Identifiers) != 1 {
		t.Errorf("expected 1 identifier, got %d", len(response.Identifiers))
	}
}

func Test_LookupAction_NonNumericAPIVersion_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.vX+json") // Non-numeric version

	w := httptest.NewRecorder()

	c.LookupAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if response.Detail != "invalid API version: X" {
		t.Errorf("expected error detail %q, got %q", "invalid API version: X", response.Detail)
	}
}

// MockErrorRepo is a mock repository that implements the Repo interface and returns a special identifier
// that can trigger JSON encoding errors
type MockErrorRepo struct {
	MockRepo
}

func (m *MockErrorRepo) Lookup(url string) []string {
	// Return a value that will definitely cause a JSON encoding error
	return []string{string([]byte{0xff, 0xfe, 0xfd}), string([]byte{0x80, 0x81, 0x82})}
}

// MockLookupRepo is a mock repository that implements the Repo interface for lookup tests
type MockLookupRepo struct {
	MockRepo
	identifiers []string
}

func (m *MockLookupRepo) Lookup(url string) []string {
	return m.identifiers
}

// errorWriter is a writer that always fails
type errorWriter struct {
	http.ResponseWriter
}

func (w *errorWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("forced write error")
}

func Test_LookupAction_JSONEncodingError_ReturnsInternalError(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Store the original logger and restore it after the test
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	mockRepo := &MockErrorRepo{}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/lookup?url=test-url", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()
	ew := &errorWriter{w}

	c.LookupAction(ew, req)

	// Even though there's an encoding error, we should still get headers
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type %s, got %s", "application/json", contentType)
	}

	contentVersion := w.Header().Get("Content-Version")
	if contentVersion != "1" {
		t.Errorf("expected Content-Version %s, got %s", "1", contentVersion)
	}

	// The error should be logged but not affect the response status
	// since headers are already written
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Verify that the error was logged
	logOutput := logBuffer.String()
	if logOutput == "" {
		t.Fatal("expected log output, but got none")
	}
	if !strings.Contains(logOutput, "Error encoding JSON") {
		t.Errorf("expected error log message containing 'Error encoding JSON', got %q", logOutput)
	}
	if !strings.Contains(logOutput, "forced write error") {
		t.Errorf("expected error message containing 'forced write error', got %q", logOutput)
	}
}
