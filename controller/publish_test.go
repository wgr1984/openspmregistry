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

func (m *mockPublishRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
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
