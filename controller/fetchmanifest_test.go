package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockRepo is a mock implementation of the repository interface
type mockRepo struct {
	getReaderFunc               func(element *models.UploadElement) (io.ReadSeekCloser, error)
	getAlternativeManifestsFunc func(element *models.UploadElement) ([]models.UploadElement, error)
	publishDateFunc             func(element *models.UploadElement) (time.Time, error)
	getSwiftToolVersionFunc     func(element *models.UploadElement) (string, error)
}

func (m *mockRepo) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	return m.getReaderFunc(element)
}

func (m *mockRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepo) Exists(element *models.UploadElement) bool {
	return true
}

func (m *mockRepo) ExtractManifestFiles(element *models.UploadElement) error {
	return nil
}

func (m *mockRepo) List(scope string, name string) ([]models.ListElement, error) {
	return nil, nil
}

func (m *mockRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	if m.getAlternativeManifestsFunc != nil {
		return m.getAlternativeManifestsFunc(element)
	}
	return nil, nil
}

func (m *mockRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	if m.publishDateFunc != nil {
		return m.publishDateFunc(element)
	}
	return time.Time{}, nil
}

func (m *mockRepo) Checksum(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
	return nil, nil
}

func (m *mockRepo) GetSwiftToolVersion(element *models.UploadElement) (string, error) {
	return m.getSwiftToolVersionFunc(element)
}

func (m *mockRepo) Lookup(url string) []string {
	return nil
}

func (m *mockRepo) Remove(element *models.UploadElement) error {
	return nil
}

// mockTimeProvider is a mock implementation of the time provider
type mockTimeProvider struct {
	currentTime time.Time
}

func (m *mockTimeProvider) Now() time.Time {
	return m.currentTime
}

// testConfig implements config.ServerConfig
type testConfig struct {
	hostname string
	port     int
	baseURL  string
	certs    config.Certs
	repo     config.Repo
	publish  config.PublishConfig
	auth     config.AuthConfig
	tls      bool
}

func newTestConfig(hostname string, port int, baseURL string) config.ServerConfig {
	return config.ServerConfig{
		Hostname:   hostname,
		Port:       port,
		Certs:      config.Certs{},
		Repo:       config.Repo{},
		Publish:    config.PublishConfig{},
		Auth:       config.AuthConfig{},
		TlsEnabled: false,
	}
}

func TestFetchManifestAction_Success(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.7\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReadSeekCloser{bytes.NewReader(manifestContent)}, nil
		},
		getAlternativeManifestsFunc: func(element *models.UploadElement) ([]models.UploadElement, error) {
			return nil, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Execute the handler
	controller.FetchManifestAction(w, req)

	// Assert response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != mimetypes.TextXSwift {
		t.Errorf("Expected Content-Type %s, got %s", mimetypes.TextXSwift, ct)
	}

	if body := w.Body.String(); string(manifestContent) != body {
		t.Errorf("Expected body %s, got %s", string(manifestContent), body)
	}

	if contentDisposition := w.Header().Get("Content-Disposition"); contentDisposition != "attachment; filename=\"Package.swift\"" {
		t.Errorf("expected Content-Disposition header to be 'attachment; filename=\"Package.swift\"', got '%s'", contentDisposition)
	}
}

func TestFetchManifestAction_WithSwiftVersion(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.8\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReadSeekCloser{bytes.NewReader(manifestContent)}, nil
		},
		getAlternativeManifestsFunc: func(element *models.UploadElement) ([]models.UploadElement, error) {
			return nil, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift?swift-version=5.8", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != mimetypes.TextXSwift {
		t.Errorf("expected Content-Type %s, got %s", mimetypes.TextXSwift, contentType)
	}

	if contentVersion := w.Header().Get("Content-Version"); contentVersion != "1" {
		t.Errorf("expected Content-Version %s, got %s", "1", contentVersion)
	}

	expectedDisposition := "attachment; filename=\"Package@swift-5.8.swift\""
	if cd := w.Header().Get("Content-Disposition"); cd != expectedDisposition {
		t.Errorf("Expected Content-Disposition %s, got %s", expectedDisposition, cd)
	}

	if body := w.Body.String(); body != string(manifestContent) {
		t.Errorf("Expected body %s, got %s", string(manifestContent), body)
	}
}

func TestFetchManifestAction_WithAlternativeManifests(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.8\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReadSeekCloser{bytes.NewReader(manifestContent)}, nil
		},
		getAlternativeManifestsFunc: func(element *models.UploadElement) ([]models.UploadElement, error) {
			manifest := models.NewUploadElement("scope", "package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
			manifest.SetFilenameOverwrite("Package@swift-5.8")
			return []models.UploadElement{*manifest}, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
		getSwiftToolVersionFunc: func(element *models.UploadElement) (string, error) {
			return "5.8", nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != mimetypes.TextXSwift {
		t.Errorf("expected Content-Type %s, got %s", mimetypes.TextXSwift, contentType)
	}

	if contentVersion := w.Header().Get("Content-Version"); contentVersion != "1" {
		t.Errorf("expected Content-Version %s, got %s", "1", contentVersion)
	}

	expectedLink := fmt.Sprintf("<%s/scope/package/1.0.0/Package.swift?swift-version=5.8>; rel=\"alternative\"; filename=\"Package@swift-5.8.swift\"; swift-tools-version=\"5.8\"", utils.BaseUrl(newTestConfig("localhost", 8080, "https://example.com")))
	if link := w.Header().Get("Link"); link != expectedLink {
		t.Errorf("expected Link header to be '%s', got '%s'", expectedLink, link)
	}

	if body := w.Body.String(); body != string(manifestContent) {
		t.Errorf("Expected body %s, got %s", string(manifestContent), body)
	}
}

func TestFetchManifestAction_NotFound(t *testing.T) {
	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return nil, fmt.Errorf("not found")
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: time.Now()},
	}

	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	controller.FetchManifestAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestFetchManifestAction_InvalidHeaders(t *testing.T) {
	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         &mockRepo{},
		timeProvider: &mockTimeProvider{currentTime: time.Now()},
	}

	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	// Don't set Accept header
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	controller.FetchManifestAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// pathParamsKey is used as a key for the context
type pathParamsKey struct{}

// mockReaderWithCloseError is a mock reader that returns an error on Close
type mockReaderWithCloseError struct {
	*bytes.Reader
}

func (m *mockReaderWithCloseError) Close() error {
	return fmt.Errorf("mock close error")
}

func TestFetchManifestAction_ReaderCloseError(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.7\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReaderWithCloseError{bytes.NewReader(manifestContent)}, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response - should still succeed despite Close error
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

func TestFetchManifestAction_PublishDateError(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.7\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReadSeekCloser{bytes.NewReader(manifestContent)}, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return time.Time{}, fmt.Errorf("mock publish date error")
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response - should still succeed with fallback to current time
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

func TestFetchManifestAction_GetAlternativeManifestsError(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.7\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReadSeekCloser{bytes.NewReader(manifestContent)}, nil
		},
		getAlternativeManifestsFunc: func(element *models.UploadElement) ([]models.UploadElement, error) {
			return nil, fmt.Errorf("mock alternative manifests error")
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response - should still succeed without Link header
	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	if link := w.Header().Get("Link"); link != "" {
		t.Errorf("expected no Link header, got '%s'", link)
	}
}

// mockReaderWithSeekError is a mock reader that returns an error on Seek
type mockReaderWithSeekError struct {
	*bytes.Reader
}

func (m *mockReaderWithSeekError) Seek(offset int64, whence int) (int64, error) {
	return 0, fmt.Errorf("mock seek error")
}

func (m *mockReaderWithSeekError) Close() error {
	return nil
}

func TestFetchManifestAction_ServeContentError(t *testing.T) {
	// Setup mock data
	manifestContent := []byte("// swift-tools-version: 5.7\nlet package = Package(name: \"test\")")
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)

	repo := &mockRepo{
		getReaderFunc: func(element *models.UploadElement) (io.ReadSeekCloser, error) {
			return &mockReaderWithSeekError{bytes.NewReader(manifestContent)}, nil
		},
		publishDateFunc: func(element *models.UploadElement) (time.Time, error) {
			return mockTime, nil
		},
	}

	controller := &Controller{
		config:       newTestConfig("localhost", 8080, "https://example.com"),
		repo:         repo,
		timeProvider: &mockTimeProvider{currentTime: mockTime},
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0/Package.swift", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+swift")

	// Add route parameters
	ctx := context.WithValue(req.Context(), pathParamsKey{}, map[string]string{
		"scope":   "scope",
		"package": "package",
		"version": "1.0.0",
	})
	req = req.WithContext(ctx)

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	controller.FetchManifestAction(w, req)

	// Check response - should fail with 500 status code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Should return a problem details object
	if contentType := w.Header().Get("Content-Type"); contentType != "application/problem+json" {
		t.Errorf("expected Content-Type %s, got %s", "application/problem+json", contentType)
	}

	if contentVersion := w.Header().Get("Content-Version"); contentVersion != "1" {
		t.Errorf("expected Content-Version %s, got %s", "1", contentVersion)
	}

	// Check the error response format
	var response struct {
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Errorf("failed to decode response body: %v", err)
	}

	expectedError := "internal server error while preparing manifest"
	if response.Detail != expectedError {
		t.Errorf("expected error message %q, got %q", expectedError, response.Detail)
	}
}
