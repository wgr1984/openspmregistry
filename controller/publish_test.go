package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

type multipartPart struct {
	name        string
	filename    string
	contentType string
	content     []byte
}

func createMultipartRequest(t *testing.T, files map[string][]byte) *http.Request {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for name, content := range files {
		var filename string
		var contentType string
		switch name {
		case string(models.SourceArchive):
			filename = "source-archive.zip"
			contentType = "application/zip"
		case string(models.SourceArchiveSignature):
			filename = "source-archive.sig"
			contentType = "application/pgp-signature"
		case string(models.Metadata):
			filename = "metadata.json"
			contentType = "application/json"
		case string(models.MetadataSignature):
			filename = "metadata.sig"
			contentType = "application/pgp-signature"
		}

		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, name, filename))
		h.Set("Content-Type", contentType)

		part, err := w.CreatePart(h)
		if err != nil {
			t.Fatalf("failed to create form part: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("failed to write form part: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/scope/package/1.0.0", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0")
	return req
}

func createMultipartRequestWithParts(t *testing.T, parts []multipartPart) *http.Request {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	for _, part := range parts {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.name, part.filename))
		h.Set("Content-Type", part.contentType)

		p, err := w.CreatePart(h)
		if err != nil {
			t.Fatalf("failed to create form part: %v", err)
		}
		if _, err := p.Write(part.content); err != nil {
			t.Fatalf("failed to write form part: %v", err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/scope/package/1.0.0", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0")
	return req
}

func Test_PublishAction_MultipleFiles_StoresAll(t *testing.T) {
	mockRepo := &mockPublishRepo{}
	ctrl := &Controller{repo: mockRepo}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive):          []byte("archive data"),
		string(models.SourceArchiveSignature): []byte("signature data"),
		string(models.Metadata):               []byte("metadata"),
		string(models.MetadataSignature):      []byte("metadata signature"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status code %d, got %d", http.StatusCreated, w.Code)
	}

	expectedFiles := map[string]string{
		"scope.package-1.0.0.zip": "archive data",
		"scope.package-1.0.0.sig": "signature data",
		"metadata.json":           "metadata",
		"metadata.sig":            "metadata signature",
	}

	for filename, expectedContent := range expectedFiles {
		if content := string(mockRepo.storedFiles[filename]); content != expectedContent {
			t.Errorf("file %s: expected content %q, got %q", filename, expectedContent, content)
		}
	}
}

func Test_PublishAction_UnsupportedUploadType_ReturnsBadRequest(t *testing.T) {
	mockRepo := &mockPublishRepo{}
	ctrl := &Controller{repo: mockRepo}

	req := createMultipartRequestWithParts(t, []multipartPart{
		{
			name:        "custom-part",
			filename:    "custom.zip",
			contentType: "application/zip",
			content:     []byte("unexpected"),
		},
	})

	w := httptest.NewRecorder()
	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	if len(mockRepo.storedFiles) != 0 {
		t.Errorf("expected no files to be stored for unsupported upload type, got %d", len(mockRepo.storedFiles))
	}
}

func Test_PublishAction_UnsupportedUploadType_CleansUpStoredElements(t *testing.T) {
	mockRepo := &mockPublishRepoWithCleanupTracking{
		storedFiles:  make(map[string][]byte),
		removedFiles: make([]string, 0),
	}
	ctrl := &Controller{repo: mockRepo}

	req := createMultipartRequestWithParts(t, []multipartPart{
		{
			name:        string(models.SourceArchive),
			filename:    "source-archive.zip",
			contentType: "application/zip",
			content:     []byte("archive data"),
		},
		{
			name:        "custom-part",
			filename:    "custom.zip",
			contentType: "application/zip",
			content:     []byte("unexpected"),
		},
	})

	w := httptest.NewRecorder()
	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	if len(mockRepo.removedFiles) == 0 {
		t.Errorf("expected stored elements to be cleaned up, none were removed")
	}

	if len(mockRepo.storedFiles) != 0 {
		t.Errorf("expected no files to remain after cleanup, found %d", len(mockRepo.storedFiles))
	}
}

func Test_PublishAction_InvalidMultipartForm_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := httptest.NewRequest("POST", "/scope/package/1.0.0", strings.NewReader("invalid form"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	req.SetPathValue("scope", "scope")
	req.SetPathValue("package", "package")
	req.SetPathValue("version", "1.0.0")
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_NoSourceArchive_ReturnsError(t *testing.T) {
	mockRepo := &mockPublishRepo{}
	ctrl := &Controller{repo: mockRepo}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.Metadata): []byte("metadata only"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func Test_PublishAction_InvalidScope_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test"),
	})
	req.SetPathValue("scope", "invalid@scope") // Invalid character in scope
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_InvalidPackage_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test"),
	})
	req.SetPathValue("package", "invalid@package") // Invalid character in package
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_WriteOperationError_ReturnsInternalError(t *testing.T) {
	ctrl := &Controller{repo: &publishWriteErrorRepo{}}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func Test_PublishAction_NilPart_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := httptest.NewRequest("POST", "/scope/package/1.0.0", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_EmptyScope_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test"),
	})
	req.SetPathValue("scope", "") // Empty scope
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_EmptyPackage_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test"),
	})
	req.SetPathValue("package", "") // Empty package name
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_PackageExists_ReturnsConflict(t *testing.T) {
	mockRepo := &mockPublishRepo{
		storedFiles: map[string][]byte{
			"scope.package-1.0.0.zip": []byte("existing package"),
		},
	}
	ctrl := &Controller{repo: mockRepo}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status code %d, got %d", http.StatusConflict, w.Code)
	}
}

func Test_PublishAction_InvalidAcceptHeader_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test"),
	})
	req.Header.Set("Accept", "invalid/type") // Invalid Accept header
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected status code %d, got %d", http.StatusUnsupportedMediaType, w.Code)
	}
}

func Test_PublishAction_InvalidMultipartForm_NoContentType_ReturnsBadRequest(t *testing.T) {
	ctrl := &Controller{}
	req := httptest.NewRequest("POST", "/scope/package/1.0.0", strings.NewReader("test"))
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_PublishAction_DebugLogging_Success(t *testing.T) {
	// Enable debug logging temporarily
	originalLogger := slog.Default()
	defer func() {
		slog.SetDefault(originalLogger)
	}()

	logBuffer := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	ctrl := &Controller{repo: &mockPublishRepo{}}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if !strings.Contains(logBuffer.String(), "Upload part") {
		t.Error("expected debug log message not found")
	}
}

func Test_PublishAction_InvalidLocationURL_ReturnsError(t *testing.T) {
	mockRepo := &urlErrorRepo{}
	ctrl := &Controller{
		repo: mockRepo,
		config: config.ServerConfig{
			Hostname: string([]byte{0x7f}), // Invalid hostname that will cause URL parsing to fail
		},
	}

	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func Test_PublishAction_GetWriterError_ReturnsError(t *testing.T) {
	ctrl := &Controller{repo: &extractErrorRepo{shouldFailGetWriter: true}}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func Test_PublishAction_WriteFailure_ReturnsInternalError(t *testing.T) {
	ctrl := &Controller{repo: &writeErrorRepo{}}
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("test data"),
	})
	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// Mock types and implementations

type mockPublishRepo struct {
	storedFiles map[string][]byte
}

func (m *mockPublishRepo) Exists(element *models.UploadElement) bool {
	_, exists := m.storedFiles[element.FileName()]
	return exists
}

func (m *mockPublishRepo) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	data, exists := m.storedFiles[element.FileName()]
	if !exists {
		return nil, fmt.Errorf("file not found")
	}
	return &mockReader{bytes.NewReader(data)}, nil
}

func (m *mockPublishRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	if m.storedFiles == nil {
		m.storedFiles = make(map[string][]byte)
	}
	var buf bytes.Buffer
	return &mockWriter{buf: &buf, filename: element.FileName(), repo: m}, nil
}

func (m *mockPublishRepo) ExtractManifestFiles(element *models.UploadElement) error {
	return nil
}

func (m *mockPublishRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	return time.Now(), nil
}

func (m *mockPublishRepo) Checksum(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepo) LoadMetadata(scope string, name string, version string) (map[string]any, error) {
	return nil, nil
}

func (m *mockPublishRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	return nil, nil
}

func (m *mockPublishRepo) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepo) Lookup(url string) []string {
	return nil
}

func (m *mockPublishRepo) Remove(element *models.UploadElement) error {
	return nil
}

func (m *mockPublishRepo) ListScopes() ([]string, error) {
	return nil, nil
}

func (m *mockPublishRepo) ListInScope(scope string) ([]models.ListElement, error) {
	return nil, nil
}

func (m *mockPublishRepo) ListAll() ([]models.ListElement, error) {
	return nil, nil
}

func (m *mockPublishRepo) LoadPackageJson(scope string, name string, version string) (map[string]any, error) {
	return nil, nil
}

func (m *mockPublishRepo) List(scope, packageName string) ([]models.ListElement, error) {
	return nil, nil
}

type mockWriter struct {
	buf      *bytes.Buffer
	filename string
	repo     *mockPublishRepo
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	return w.buf.Write(p)
}

func (w *mockWriter) Close() error {
	w.repo.storedFiles[w.filename] = w.buf.Bytes()
	return nil
}

type mockReader struct {
	*bytes.Reader
}

func (r *mockReader) Close() error {
	return nil
}

type publishErrorWriter struct {
	shouldFail bool
}

func (w *publishErrorWriter) Write(p []byte) (n int, err error) {
	if w.shouldFail {
		return 0, fmt.Errorf("write error")
	}
	return len(p), nil
}

func (w *publishErrorWriter) Close() error {
	return nil
}

type publishWriteErrorRepo struct {
	mockPublishRepo
}

func (r *publishWriteErrorRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return &publishErrorWriter{shouldFail: true}, nil
}

type urlErrorRepo struct {
	mockPublishRepo
	element *models.UploadElement
}

func (r *urlErrorRepo) Exists(element *models.UploadElement) bool {
	r.element = element
	return false
}

type extractErrorRepo struct {
	mockPublishRepo
	shouldFailGetWriter bool
}

func (r *extractErrorRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	if r.shouldFailGetWriter {
		return nil, fmt.Errorf("get writer error")
	}
	return &mockWriter{buf: &bytes.Buffer{}, filename: element.FileName(), repo: &r.mockPublishRepo}, nil
}

type writeErrorRepo struct {
	mockPublishRepo
}

func (r *writeErrorRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return &publishWriteFailWriter{shouldFail: true}, nil
}

type publishWriteFailWriter struct {
	shouldFail bool
}

func (w *publishWriteFailWriter) Write(p []byte) (n int, err error) {
	if w.shouldFail {
		return 0, fmt.Errorf("write error")
	}
	return len(p), nil
}

func (w *publishWriteFailWriter) Close() error {
	return nil
}

// Test that all stored elements are cleaned up when Package.json validation fails
func Test_PublishAction_RequirePackageJsonFails_CleansUpAllElements(t *testing.T) {
	mockRepo := &mockPublishRepoWithCleanupTracking{
		storedFiles:       make(map[string][]byte),
		removedFiles:      make([]string, 0),
		existsPackageJson: false,
	}

	ctrl := &Controller{
		repo: mockRepo,
		config: config.ServerConfig{
			PackageCollections: config.PackageCollectionsConfig{
				RequirePackageJson: true,
			},
		},
	}

	// Create a request with multiple files
	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive):          []byte("archive data"),
		string(models.SourceArchiveSignature): []byte("signature data"),
		string(models.Metadata):               []byte("metadata"),
		string(models.MetadataSignature):      []byte("metadata signature"),
	})

	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	// Should fail with 422 Unprocessable Entity
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("Expected status code %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}

	// Verify error message
	if !strings.Contains(w.Body.String(), "Package.json is required but not found") {
		t.Errorf("Expected error message about Package.json, got: %s", w.Body.String())
	}

	// Log what was actually removed for debugging
	t.Logf("Stored files: %v", mockRepo.storedFiles)
	t.Logf("Removed files: %v", mockRepo.removedFiles)

	// Verify all uploaded files were cleaned up
	// The filenames come from UploadElement.FileName() which generates names based on scope.name-version
	expectedMinimumCleanups := 4 // source-archive, source-archive.sig, metadata, metadata.sig

	// Verify at least the basic files were attempted to be removed
	if len(mockRepo.removedFiles) < expectedMinimumCleanups {
		t.Errorf("Expected at least %d files to be cleaned up, got %d: %v", expectedMinimumCleanups, len(mockRepo.removedFiles), mockRepo.removedFiles)
	}

	// Verify that storedFiles is empty after cleanup
	if len(mockRepo.storedFiles) > 0 {
		t.Errorf("Expected all stored files to be removed, but %d remain: %v", len(mockRepo.storedFiles), mockRepo.storedFiles)
	}
}

// Test that cleanup doesn't happen when Package.json validation passes
func Test_PublishAction_RequirePackageJsonSucceeds_NoCleanup(t *testing.T) {
	mockRepo := &mockPublishRepoWithCleanupTracking{
		storedFiles:       make(map[string][]byte),
		removedFiles:      make([]string, 0),
		existsPackageJson: true,
	}

	ctrl := &Controller{
		repo: mockRepo,
		config: config.ServerConfig{
			PackageCollections: config.PackageCollectionsConfig{
				RequirePackageJson: true,
			},
		},
	}

	req := createMultipartRequest(t, map[string][]byte{
		string(models.SourceArchive): []byte("archive data"),
	})

	w := httptest.NewRecorder()

	ctrl.PublishAction(w, req)

	// Should succeed
	if w.Code != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, w.Code)
	}

	// Verify no files were removed
	if len(mockRepo.removedFiles) > 0 {
		t.Errorf("Expected no files to be removed, but %d were removed: %v", len(mockRepo.removedFiles), mockRepo.removedFiles)
	}
}

// mockPublishRepoWithCleanupTracking tracks which files are stored and removed
type mockPublishRepoWithCleanupTracking struct {
	storedFiles       map[string][]byte
	removedFiles      []string
	existsPackageJson bool
}

func (m *mockPublishRepoWithCleanupTracking) Exists(element *models.UploadElement) bool {
	// Special handling for Package.json
	if element.FileName() == "Package.json" {
		return m.existsPackageJson
	}
	_, exists := m.storedFiles[element.FileName()]
	return exists
}

func (m *mockPublishRepoWithCleanupTracking) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	if m.storedFiles == nil {
		m.storedFiles = make(map[string][]byte)
	}
	var buf bytes.Buffer
	return &mockWriterWithCleanupTracking{buf: &buf, filename: element.FileName(), repo: m}, nil
}

func (m *mockPublishRepoWithCleanupTracking) Remove(element *models.UploadElement) error {
	m.removedFiles = append(m.removedFiles, element.FileName())
	delete(m.storedFiles, element.FileName())
	return nil
}

func (m *mockPublishRepoWithCleanupTracking) Write(filename string, data []byte) {
	m.storedFiles[filename] = data
}

func (m *mockPublishRepoWithCleanupTracking) ExtractManifestFiles(element *models.UploadElement) error {
	return nil
}

func (m *mockPublishRepoWithCleanupTracking) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	return []models.UploadElement{}, nil
}

func (m *mockPublishRepoWithCleanupTracking) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) EncodeBase64(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepoWithCleanupTracking) PublishDate(element *models.UploadElement) (time.Time, error) {
	return time.Now(), nil
}

func (m *mockPublishRepoWithCleanupTracking) Checksum(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepoWithCleanupTracking) LoadMetadata(scope string, name string, version string) (map[string]any, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	return "", nil
}

func (m *mockPublishRepoWithCleanupTracking) Lookup(url string) []string {
	return nil
}

func (m *mockPublishRepoWithCleanupTracking) ListScopes() ([]string, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) ListInScope(scope string) ([]models.ListElement, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) ListAll() ([]models.ListElement, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) LoadPackageJson(scope string, name string, version string) (map[string]any, error) {
	return nil, nil
}

func (m *mockPublishRepoWithCleanupTracking) List(scope, packageName string) ([]models.ListElement, error) {
	return nil, nil
}

type mockWriterWithCleanupTracking struct {
	buf      *bytes.Buffer
	filename string
	repo     *mockPublishRepoWithCleanupTracking
}

func (m *mockWriterWithCleanupTracking) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockWriterWithCleanupTracking) Close() error {
	m.repo.Write(m.filename, m.buf.Bytes())
	return nil
}
