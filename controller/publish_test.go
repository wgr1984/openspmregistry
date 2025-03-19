package controller

import (
	"OpenSPMRegistry/models"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

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
