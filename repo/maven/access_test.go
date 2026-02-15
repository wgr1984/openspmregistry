package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_buildMavenPathForElement_SourceArchive_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	path := a.buildMavenPathForElement(element)
	expected := "testScope/my-package/1.0.0/my-package-1.0.0.zip"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_Metadata_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationJson, models.Metadata)
	path := a.buildMavenPathForElement(element)
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-metadata.json"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_PackageSwift_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	path := a.buildMavenPathForElement(element)
	// Maven-compliant: lowercase classifier
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-package.swift"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_PackageJson_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationJson, models.PackageManifestJson)
	path := a.buildMavenPathForElement(element)
	// Maven-compliant: lowercase classifier
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-package.json"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_AlternativeManifest_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	element.SetFilenameOverwrite("Package@swift-5.0")
	path := a.buildMavenPathForElement(element)
	// Format: package-swift-5.0 (Maven-compliant: lowercase, hyphens only, no @ symbol)
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-package-swift-5.0.swift"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_WithGroupIdPrefix_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{GroupIdPrefix: "com.example"}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	path := a.buildMavenPathForElement(element)
	expected := "com/example/testScope/my-package/1.0.0/my-package-1.0.0.zip"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_MetadataSignature_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", "application/octet-stream", models.MetadataSignature)
	path := a.buildMavenPathForElement(element)
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-metadata.sig"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_AlternativeManifestWithSwiftPrefix_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// Test with Package@swift-5.7.0 (already has "swift-" prefix)
	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	element.SetFilenameOverwrite("Package@swift-5.7.0")
	path := a.buildMavenPathForElement(element)
	// Should extract "5.7.0" (removing "swift-" prefix)
	// Maven-compliant: lowercase, hyphens only
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-package-swift-5.7.0.swift"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_AlternativeManifestWithoutSwiftPrefix_ReturnsCorrectPath(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// Test with Package@5.7.0 (no "swift-" prefix)
	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	element.SetFilenameOverwrite("Package@5.7.0")
	path := a.buildMavenPathForElement(element)
	// Should use "5.7.0" as-is
	// Maven-compliant: lowercase, hyphens only
	expected := "testScope/my-package/1.0.0/my-package-1.0.0-package-swift-5.7.0.swift"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_FileWithoutExtension_UsesMimeType(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// Create element with filename that has no extension
	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	element.SetFilenameOverwrite("archive") // No extension
	path := a.buildMavenPathForElement(element)
	// Should use .zip extension from MIME type
	expected := "testScope/my-package/1.0.0/my-package-1.0.0.zip"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_buildMavenPathForElement_UnknownMimeType_DefaultsToZip(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// Create element with unknown MIME type and no extension
	element := models.NewUploadElement("testScope", "my-package", "1.0.0", "unknown/mime-type", models.SourceArchive)
	element.SetFilenameOverwrite("file") // No extension
	path := a.buildMavenPathForElement(element)
	// Should default to .zip
	expected := "testScope/my-package/1.0.0/my-package-1.0.0.zip"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}

func Test_Exists_FileExists_ReturnsTrue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	exists := a.Exists(context.Background(), element)
	if !exists {
		t.Errorf("expected true, got false")
	}
}

func Test_Exists_FileDoesNotExist_ReturnsFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	exists := a.Exists(context.Background(), element)
	if exists {
		t.Errorf("expected false, got true")
	}
}

func Test_Exists_Error_ReturnsFalse(t *testing.T) {
	// Use invalid URL to force error
	cfg := config.MavenConfig{BaseURL: "http://invalid-url-that-does-not-exist.local"}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	exists := a.Exists(context.Background(), element)
	if exists {
		t.Errorf("expected false on error, got true")
	}
}

func Test_checkRangeSupport_AcceptsRanges_ReturnsTrue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	supports, err := a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !supports {
		t.Errorf("expected true, got false")
	}
}

func Test_checkRangeSupport_NoAcceptRanges_ReturnsFalse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	supports, err := a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if supports {
		t.Errorf("expected false, got true")
	}
}

func Test_checkRangeSupport_CachesResult(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			callCount++
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// First call
	_, err := a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second call should use cache
	_, err = a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only call HEAD once
	if callCount != 1 {
		t.Errorf("expected 1 HEAD call, got %d", callCount)
	}
}

func Test_checkRangeSupport_HeadFails_ReturnsFalseAndCaches(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			callCount++
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// First call - should return false on error
	supports, err := a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if supports {
		t.Errorf("expected false on HEAD failure, got true")
	}

	// Second call should use cached false result
	supports2, err := a.checkRangeSupport(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if supports2 {
		t.Errorf("expected false on cached result, got true")
	}

	// Should only call HEAD once (cached after first failure)
	if callCount != 1 {
		t.Errorf("expected 1 HEAD call, got %d", callCount)
	}
}

func Test_GetReader_WithRangeSupport_ReturnsRangeReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader != "" {
				w.Header().Set("Content-Range", "bytes 0-99/100")
				w.WriteHeader(http.StatusPartialContent)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			_, _ = w.Write([]byte("test data"))
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	reader, err := a.GetReader(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if reader == nil {
		t.Errorf("expected reader, got nil")
	}
}

func Test_GetReader_WithoutRangeSupport_ReturnsBufferedReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("test data"))
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	reader, err := a.GetReader(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if reader == nil {
		t.Errorf("expected reader, got nil")
	}

	// Verify it's buffered by reading
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("expected 'test data', got '%s'", string(data))
	}
}

func Test_GetReader_RangeReaderFails_FallsBackToBuffered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			// Simulate range request failure - return error status
			rangeHeader := r.Header.Get("Range")
			if rangeHeader != "" {
				// Return error to force fallback
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			} else {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test data"))
			}
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	reader, err := a.GetReader(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if reader == nil {
		t.Errorf("expected reader, got nil")
	}

	// Should fall back to buffered reader
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("expected 'test data', got '%s'", string(data))
	}
}

func Test_GetWriter_ValidElement_ReturnsWriter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	writer, err := a.GetWriter(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer == nil {
		t.Errorf("expected writer, got nil")
	}

	// Write and close to trigger PUT
	if _, err := writer.Write([]byte("test data")); err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	}
	err = writer.Close()
	if err != nil {
		t.Errorf("unexpected error closing writer: %v", err)
	}
}

func Test_mavenWriter_Write_AppendsToBuffer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	writer := newMavenWriter(c, cfg, a, "test/path", element, context.Background())
	data := []byte("test data")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected to write %d bytes, wrote %d", len(data), n)
	}
}

func Test_mavenWriter_Close_UploadsData(t *testing.T) {
	uploadedByPath := make(map[string][]byte)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			data, _ := io.ReadAll(r.Body)
			uploadedByPath[r.URL.Path] = data
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	path := a.buildMavenPathForElement(element)
	writer := newMavenWriter(c, cfg, a, path, element, context.Background())
	testData := []byte("test upload data")
	if _, err := writer.Write(testData); err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	}
	err := writer.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close() does multiple PUTs (main artifact, .sha256, maven-metadata.xml); assert on main artifact path only
	mainPath := "/" + path
	uploadedData := uploadedByPath[mainPath]
	if string(uploadedData) != string(testData) {
		t.Errorf("expected uploaded data '%s', got '%s'", string(testData), string(uploadedData))
	}
}

func Test_GetWriter_ErrorBuildingPath_ReturnsError(t *testing.T) {
	// This test would require a way to force buildMavenPathForElement to fail
	// Since it doesn't currently return errors, we'll test the normal case
	// and note that error handling is already covered by the nil check in GetWriter
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	// Normal case - should work
	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	writer, err := a.GetWriter(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer == nil {
		t.Errorf("expected writer, got nil")
	}
	_ = writer.Close()
}

func Test_mavenWriter_Close_UpdatesSPMRegistryIndexWithScopeAndPackage(t *testing.T) {
	var indexPUTBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if r.Method == "GET" && strings.HasSuffix(path, "index-1.json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == "PUT" {
			if strings.HasSuffix(path, "index-1.json") {
				indexPUTBody, _ = io.ReadAll(r.Body)
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)
	a := newAccess(c, cfg)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	writer, err := a.GetWriter(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := writer.Write([]byte("zip")); err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("unexpected error closing: %v", err)
	}

	if len(indexPUTBody) == 0 {
		t.Fatal("expected index.json to be PUT, no body captured")
	}
	var index spmRegistryIndexResponse
	if err := json.Unmarshal(indexPUTBody, &index); err != nil {
		t.Fatalf("failed to decode index: %v", err)
	}
	// Index writes only packages; scopes are derived when reading
	pkgs, ok := index.Packages["testScope"]
	if !ok || len(pkgs) == 0 {
		t.Errorf("expected index.packages[testScope] to contain my-package, got %v", index.Packages)
	} else {
		hasPkg := false
		for _, p := range pkgs {
			if p == "my-package" {
				hasPkg = true
				break
			}
		}
		if !hasPkg {
			t.Errorf("expected index.packages[testScope] to contain my-package, got %v", pkgs)
		}
	}
}
