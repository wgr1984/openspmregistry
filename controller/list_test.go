package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type MockListRepo struct {
	MockRepo
	elements []models.ListElement
	err      error
}

func Test_ListAction_MissingAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package", nil)
	w := httptest.NewRecorder()

	c.ListAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_ListAction_InvalidAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")
	w := httptest.NewRecorder()

	c.ListAction(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status code %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
}

func Test_ListAction_Success(t *testing.T) {
	elements := []models.ListElement{
		{Scope: "scope", PackageName: "package", Version: "1.0.0"},
		{Scope: "scope", PackageName: "package", Version: "2.0.0"},
	}
	mockRepo := &MockListRepo{elements: elements}
	cfg := config.ServerConfig{Hostname: "localhost", Port: 8080}
	c := NewController(cfg, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.ListAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response struct {
		Releases map[string]models.Release `json:"releases"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Releases) != len(elements) {
		t.Errorf("expected %d releases, got %d", len(elements), len(response.Releases))
	}
	if link := w.Header().Get("Link"); !strings.Contains(link, "latest-version") {
		t.Errorf("expected Link header with latest-version, got %q", link)
	}
}

func Test_ListAction_Pagination_ReturnsLinkHeaders(t *testing.T) {
	elements := []models.ListElement{
		{Scope: "scope", PackageName: "package", Version: "1.0.0"},
		{Scope: "scope", PackageName: "package", Version: "1.1.0"},
		{Scope: "scope", PackageName: "package", Version: "2.0.0"},
	}
	mockRepo := &MockListRepo{elements: elements}
	cfg := config.ServerConfig{Hostname: "localhost", Port: 8080, ListPageSize: 1}
	c := NewController(cfg, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package?page=1", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.ListAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}
	link := w.Header().Get("Link")
	for _, rel := range []string{"first", "next", "last"} {
		if !strings.Contains(link, "rel=\""+rel+"\"") {
			t.Errorf("expected Link header with %q, got %q", rel, link)
		}
	}
	var response struct {
		Releases map[string]models.Release `json:"releases"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// Page 1 per_page=1 should return highest precedence (2.0.0)
	if _, ok := response.Releases["2.0.0"]; !ok {
		t.Errorf("expected page 1 to return 2.0.0 (highest precedence), got %v", response.Releases)
	}
	if len(response.Releases) != 1 {
		t.Errorf("expected 1 release per page, got %d", len(response.Releases))
	}
}

func Test_ListAction_JSONEncodingError_LogsError(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	elements := []models.ListElement{{Version: "1.0.0"}}
	mockRepo := &MockListRepo{elements: elements}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()
	ew := &errorWriter{ResponseWriter: w}

	c.ListAction(ew, req)

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Error encoding JSON") {
		t.Errorf("expected error log message containing 'Error encoding JSON', got %q", logOutput)
	}
	if !strings.Contains(logOutput, "forced write error") {
		t.Errorf("expected error message containing 'forced write error', got %q", logOutput)
	}
}

func (m *MockListRepo) List(ctx context.Context, scope string, name string) ([]models.ListElement, error) {
	return m.elements, m.err
}
