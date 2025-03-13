package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
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

func (m *MockListRepo) List(scope string, name string) ([]models.ListElement, error) {
	return m.elements, m.err
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
		{Version: "1.0.0"},
		{Version: "2.0.0"},
	}
	mockRepo := &MockListRepo{elements: elements}
	c := NewController(config.ServerConfig{}, mockRepo)

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
