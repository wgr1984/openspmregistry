package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_DownloadSourceArchiveAction_MissingAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	expectedBody := `{"detail":"missing Accept header"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_DownloadSourceArchiveAction_InvalidAcceptHeader_ReturnsBadRequest(t *testing.T) {
	c := NewController(config.ServerConfig{}, nil)
	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status code %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}

	expectedBody := `{"detail":"unsupported media type: json"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_DownloadSourceArchiveAction_ArchiveNotFound_ReturnsNotFound(t *testing.T) {
	mockRepo := &MockDownloadRepo{exists: false}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status code %d, got %d", http.StatusNotFound, w.Code)
	}

	expectedBody := `{"detail":"source archive scope.package-1.0.0.zip does not exist"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_DownloadSourceArchiveAction_Success(t *testing.T) {
	content := "test archive content"
	mockRepo := &MockDownloadRepo{
		exists:   true,
		checksum: "test-checksum",
		reader:   strings.NewReader(content),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check headers
	headers := w.Header()
	expectedHeaders := map[string]string{
		"Content-Type":    mimetypes.ApplicationZip,
		"Content-Version": "1",
		"Cache-Control":   "public, immutable",
		"Digest":          "sha-256=test-checksum",
		"Accept-Ranges":   "bytes",
	}
	for key, expected := range expectedHeaders {
		if got := headers.Get(key); got != expected {
			t.Errorf("expected header %s to be %q, got %q", key, expected, got)
		}
	}

	// Check Content-Disposition format
	cd := headers.Get("Content-Disposition")
	expectedCDPrefix := `attachment; filename="scope.package-1.0.0.zip"`
	if !strings.HasPrefix(cd, expectedCDPrefix) {
		t.Errorf("expected Content-Disposition to start with %q, got %q", expectedCDPrefix, cd)
	}

	// Check content
	if body := w.Body.String(); body != content {
		t.Errorf("expected body %q, got %q", content, body)
	}
}

func Test_DownloadSourceArchiveAction_WithSignature(t *testing.T) {
	mockRepo := &MockDownloadRepo{
		exists:    true,
		checksum:  "test-checksum",
		signature: "test-signature",
		reader:    strings.NewReader("content"),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if sig := w.Header().Get("X-Swift-Package-Signature"); sig != "test-signature" {
		t.Errorf("expected signature %s, got %s", "test-signature", sig)
	}
	if fmt := w.Header().Get("X-Swift-Package-Signature-Format"); fmt != "cms-1.0.0" {
		t.Errorf("expected signature format %s, got %s", "cms-1.0.0", fmt)
	}
}

func Test_DownloadSourceArchiveAction_ChecksumError_LogsError(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	mockRepo := &MockDownloadRepo{
		exists:      true,
		checksumErr: fmt.Errorf("checksum error"),
		reader:      strings.NewReader("content"),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Error calculating checksum") {
		t.Errorf("expected error log message containing 'Error calculating checksum', got %q", logOutput)
	}
}

func Test_DownloadSourceArchiveAction_ReaderError_ReturnsError(t *testing.T) {
	mockRepo := &MockDownloadRepo{
		exists:    true,
		readerErr: fmt.Errorf("reader error"),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}

	expectedBody := `{"detail":"error reading source archive scope.package-1.0.0.zip"}`
	if body := strings.TrimSpace(w.Body.String()); body != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, body)
	}
}

func Test_DownloadSourceArchiveAction_ReaderError_DoesNotPanic(t *testing.T) {
	mockRepo := &MockDownloadRepo{
		exists:    true,
		readerErr: fmt.Errorf("reader error"),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	// This should not panic even though reader is nil
	c.DownloadSourceArchiveAction(w, req)
}

func Test_DownloadSourceArchiveAction_ByteRangeRequest_ReturnsPartialContent(t *testing.T) {
	content := "test archive content"
	mockRepo := &MockDownloadRepo{
		exists:   true,
		checksum: "test-checksum",
		reader:   strings.NewReader(content),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	req.Header.Set("Range", "bytes=5-10")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusPartialContent {
		t.Errorf("expected status code %d, got %d", http.StatusPartialContent, w.Code)
	}

	expectedContent := content[5:11] // Range is inclusive
	if body := w.Body.String(); body != expectedContent {
		t.Errorf("expected body %q, got %q", expectedContent, body)
	}

	if cr := w.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 5-10/") {
		t.Errorf("expected Content-Range to start with 'bytes 5-10/', got %q", cr)
	}
}

func Test_DownloadSourceArchiveAction_SignatureError_LogsAndContinues(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	mockRepo := &MockDownloadRepo{
		exists:       true,
		checksum:     "test-checksum",
		reader:       strings.NewReader("content"),
		signatureErr: fmt.Errorf("signature error"),
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Signature not found") {
		t.Errorf("expected log message containing 'Signature not found', got %q", logOutput)
	}

	// Signature headers should not be set
	if sig := w.Header().Get("X-Swift-Package-Signature"); sig != "" {
		t.Errorf("expected no signature header, got %q", sig)
	}
	if fmt := w.Header().Get("X-Swift-Package-Signature-Format"); fmt != "" {
		t.Errorf("expected no signature format header, got %q", fmt)
	}
}

func Test_DownloadSourceArchiveAction_CloseError_LogsError(t *testing.T) {
	var logBuffer strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(originalLogger)

	// Create a custom mock repo that returns a reader with a Close error
	mockRepo := &MockDownloadRepo{
		exists:   true,
		checksum: "test-checksum",
		reader:   &errorCloser{strings.NewReader("content")},
	}
	c := NewController(config.ServerConfig{}, mockRepo)

	req := httptest.NewRequest("GET", "/scope/package/1.0.0.zip", nil)
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0.zip")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")
	w := httptest.NewRecorder()

	c.DownloadSourceArchiveAction(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status code %d, got %d", http.StatusOK, w.Code)
	}

	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Error closing reader") {
		t.Errorf("expected log message containing 'Error closing reader', got %q", logOutput)
	}
}

// Mock types and implementations

type MockDownloadRepo struct {
	MockRepo
	exists       bool
	checksum     string
	checksumErr  error
	signature    string
	signatureErr error
	reader       io.ReadSeeker
	readerErr    error
}

func (m *MockDownloadRepo) Exists(element *models.UploadElement) bool {
	return m.exists
}

func (m *MockDownloadRepo) Checksum(element *models.UploadElement) (string, error) {
	return m.checksum, m.checksumErr
}

func (m *MockDownloadRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return m.signature, m.signatureErr
}

func (m *MockDownloadRepo) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	if m.readerErr != nil {
		return nil, m.readerErr
	}
	if closer, ok := m.reader.(io.ReadSeekCloser); ok {
		return closer, nil
	}
	return &mockReadSeekCloser{m.reader}, nil
}

type mockReadSeekCloser struct {
	io.ReadSeeker
}

func (m *mockReadSeekCloser) Close() error {
	return nil
}

type errorCloser struct {
	io.ReadSeeker
}

func (e *errorCloser) Close() error {
	return fmt.Errorf("close error")
}
