package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_InfoAction_MissingAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	expectedBody := `{"detail":"missing Accept header"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_InfoAction_InvalidAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status code %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}

	expectedBody := `{"detail":"unsupported media type: zip"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_InfoAction_ArchiveNotFound_ReturnsNotFound(t *testing.T) {
	mockRepo := &MockInfoRepo{exists: false}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, w.Code)
	}

	expectedBody := `{"detail":"source archive scope.package-1.0.0.zip does not exist"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_InfoAction_Success(t *testing.T) {
	publishDate := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	mockRepo := &MockInfoRepo{
		exists:      true,
		metadata:    map[string]interface{}{"key": "value"},
		signature:   "test-signature",
		publishDate: publishDate,
		checksum:    "test-checksum",
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check headers
	headers := w.Header()
	expectedHeaders := map[string]string{
		"Content-Type":    mimetypes.ApplicationJson,
		"Content-Version": "1",
	}
	for key, expected := range expectedHeaders {
		if got := headers.Get(key); got != expected {
			t.Errorf("expected header %s to be %q, got %q", key, expected, got)
		}
	}

	// Parse and verify response body
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	expectedResponse := map[string]interface{}{
		"id":      "scope.package",
		"version": "1.0.0",
		"resources": []interface{}{
			map[string]interface{}{
				"name":     "source-archive",
				"type":     "application/zip",
				"checksum": "test-checksum",
				"signing": map[string]interface{}{
					"signatureBase64Encoded": "test-signature",
					"signatureFormat":        "cms-1.0.0",
				},
			},
		},
		"metadata":    map[string]interface{}{"key": "value"},
		"publishedAt": "2024-03-15T12:00:00Z",
	}

	// Compare response fields
	if response["id"] != expectedResponse["id"] {
		t.Errorf("expected id %q, got %q", expectedResponse["id"], response["id"])
	}
	if response["version"] != expectedResponse["version"] {
		t.Errorf("expected version %q, got %q", expectedResponse["version"], response["version"])
	}
	if response["publishedAt"] != expectedResponse["publishedAt"] {
		t.Errorf("expected publishedAt %q, got %q", expectedResponse["publishedAt"], response["publishedAt"])
	}
}

func Test_InfoAction_MetadataError_LogsAndContinues(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	fixedTime := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	mockTimeProvider := utils.NewMockTimeProvider(fixedTime)

	mockRepo := &MockInfoRepo{
		exists:      true,
		metadataErr: fmt.Errorf("metadata error"),
		publishDate: fixedTime,
	}
	c := &Controller{
		repo:         mockRepo,
		timeProvider: mockTimeProvider,
	}

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Error fetching metadata") {
		t.Errorf("expected log message containing 'Error fetching metadata', got %q", logOutput)
	}

	// Verify empty metadata in response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	metadata, ok := response["metadata"].(map[string]interface{})
	if !ok {
		t.Error("expected metadata to be an empty object")
	} else if len(metadata) != 0 {
		t.Errorf("expected empty metadata, got %v", metadata)
	}
}

func Test_InfoAction_SignatureError_LogsAndContinues(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	fixedTime := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	mockTimeProvider := utils.NewMockTimeProvider(fixedTime)

	mockRepo := &MockInfoRepo{
		exists:       true,
		signatureErr: fmt.Errorf("signature error"),
		publishDate:  fixedTime,
	}
	c := &Controller{
		repo:         mockRepo,
		timeProvider: mockTimeProvider,
	}

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Signature not found") {
		t.Errorf("expected log message containing 'Signature not found', got %q", logOutput)
	}

	// Verify null signing in response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	resources, ok := response["resources"].([]interface{})
	if !ok || len(resources) == 0 {
		t.Fatal("expected non-empty resources array")
	}

	resource, ok := resources[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected resource to be an object")
	}

	if resource["signing"] != nil {
		t.Errorf("expected signing to be null, got %v", resource["signing"])
	}
}

func Test_InfoAction_ChecksumError_LogsAndContinues(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	fixedTime := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	mockTimeProvider := utils.NewMockTimeProvider(fixedTime)

	mockRepo := &MockInfoRepo{
		exists:      true,
		checksumErr: fmt.Errorf("checksum error"),
		publishDate: fixedTime,
	}
	c := &Controller{
		repo:         mockRepo,
		timeProvider: mockTimeProvider,
	}

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.json", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.InfoAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Checksum error") {
		t.Errorf("expected log message containing 'Checksum error', got %q", logOutput)
	}

	// Verify empty checksum in response
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	resources, ok := response["resources"].([]interface{})
	if !ok || len(resources) == 0 {
		t.Fatal("expected non-empty resources array")
	}

	resource, ok := resources[0].(map[string]interface{})
	if !ok {
		t.Fatal("expected resource to be an object")
	}

	if resource["checksum"] != "" {
		t.Errorf("expected empty checksum, got %v", resource["checksum"])
	}
}

func Test_InfoAction_PublishDateError_UsesCurrentTime(t *testing.T) {
	// Create a fixed time for testing
	fixedTime := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
	mockTimeProvider := utils.NewMockTimeProvider(fixedTime)

	// Setup controller with mock time provider
	ctrl := &Controller{
		repo: &MockInfoRepo{
			exists:         true,
			publishDateErr: fmt.Errorf("publish date error"),
		},
		timeProvider: mockTimeProvider,
	}

	// Setup request
	req := httptest.NewRequest(http.MethodGet, "/scope/package/1.0.0.json", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.json")

	// Setup response recorder
	w := httptest.NewRecorder()

	// Call the handler
	ctrl.InfoAction(w, req)

	// Check response
	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	publishedAt, ok := result["publishedAt"].(string)
	if !ok {
		t.Fatal("expected publishedAt to be a string")
	}

	// Parse the publishedAt time
	publishTime, err := time.Parse(time.RFC3339, publishedAt)
	if err != nil {
		t.Fatalf("failed to parse publishedAt time: %v", err)
	}

	// Compare with fixed time
	if !publishTime.Equal(fixedTime.UTC()) {
		t.Errorf("expected publishedAt %v, got %v", fixedTime.UTC(), publishTime)
	}
}

// Mock types and implementations

type MockInfoRepo struct {
	MockRepo
	exists         bool
	metadata       map[string]interface{}
	metadataErr    error
	signature      string
	signatureErr   error
	publishDate    time.Time
	publishDateErr error
	checksum       string
	checksumErr    error
}

func (m *MockInfoRepo) Exists(element *models.UploadElement) bool {
	return m.exists
}

func (m *MockInfoRepo) FetchMetadata(scope, packageName, version string) (map[string]interface{}, error) {
	return m.metadata, m.metadataErr
}

func (m *MockInfoRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return m.signature, m.signatureErr
}

func (m *MockInfoRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	return m.publishDate, m.publishDateErr
}

func (m *MockInfoRepo) Checksum(element *models.UploadElement) (string, error) {
	return m.checksum, m.checksumErr
}

func (m *MockInfoRepo) List(scope, packageName string) ([]models.ListElement, error) {
	if m.exists {
		return []models.ListElement{
			{Scope: scope, PackageName: packageName, Version: "1.0.0"},
		}, nil
	}
	return nil, nil
}
