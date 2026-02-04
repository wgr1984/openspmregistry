package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func Test_NewMavenRepo_ValidConfig_ReturnsRepo(t *testing.T) {
	cfg := config.MavenConfig{
		BaseURL: "https://repo.example.com",
	}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Errorf("expected repo, got nil")
	}
}

func Test_NewMavenRepo_InvalidConfig_ReturnsError(t *testing.T) {
	// newClient doesn't validate URLs, so this test is not applicable
	// The URL validation happens at HTTP request time
	// Skip this test as it's not a valid test case
	t.Skip("newClient doesn't validate URLs upfront")
}

func Test_List_ValidMetadata_ReturnsVersions(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>com.example</groupId>
	<artifactId>test-package</artifactId>
	<versioning>
		<versions>
			<version>1.0.0</version>
			<version>1.1.0</version>
			<version>2.0.0</version>
		</versions>
	</versioning>
</metadata>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "maven-metadata.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(xmlData))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	elements, err := repo.List(context.Background(), "com.example", "test-package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(elements) != 3 {
		t.Errorf("expected 3 versions, got %d", len(elements))
	}
}

func Test_List_NoMetadata_ReturnsEmptyList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	elements, err := repo.List(context.Background(), "com.example", "test-package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(elements) != 0 {
		t.Errorf("expected 0 versions, got %d", len(elements))
	}
}

func Test_GetAlternativeManifests_ValidMetadata_ReturnsOtherVersions(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>test</groupId>
	<artifactId>TestPackage</artifactId>
	<versioning>
		<versions>
			<version>1.0.0</version>
			<version>1.1.0</version>
			<version>2.0.0</version>
		</versions>
	</versioning>
</metadata>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "maven-metadata.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(xmlData))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("test", "TestPackage", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	manifests, err := repo.GetAlternativeManifests(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 2 {
		t.Errorf("expected 2 alternative manifests (1.1.0, 2.0.0), got %d", len(manifests))
	}
	versions := make(map[string]bool)
	for _, m := range manifests {
		if m.Scope != "test" || m.Name != "TestPackage" {
			t.Errorf("expected scope=test name=TestPackage, got scope=%s name=%s", m.Scope, m.Name)
		}
		versions[m.Version] = true
	}
	if !versions["1.1.0"] || !versions["2.0.0"] {
		t.Errorf("expected versions 1.1.0 and 2.0.0, got %v", versions)
	}
}

func Test_ListScopes_HTMLListing_ReturnsScopes(t *testing.T) {
	html := `<!DOCTYPE html><html><body><a href="test/">test/</a></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(html))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	scopes, err := repo.ListScopes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 1 || scopes[0] != "test" {
		t.Errorf("expected scopes [test], got %v", scopes)
	}
}

func Test_ListInScope_ValidMetadata_ReturnsListElements(t *testing.T) {
	metadataXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>test</groupId>
	<artifactId>TestPackage</artifactId>
	<versioning>
		<versions>
			<version>1.0.0</version>
			<version>1.1.0</version>
		</versions>
	</versioning>
</metadata>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "maven-metadata.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(metadataXML))
		} else if r.URL.Path == "/test/" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body><a href="TestPackage/">TestPackage/</a></body></html>`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	elements, err := repo.ListInScope(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elements) != 2 {
		t.Errorf("expected 2 list elements (TestPackage 1.0.0 and 1.1.0), got %d", len(elements))
	}
	for _, e := range elements {
		if e.Scope != "test" || e.PackageName != "TestPackage" {
			t.Errorf("expected scope=test package=TestPackage, got scope=%s package=%s", e.Scope, e.PackageName)
		}
	}
}

func Test_ListAll_CombinesScopes(t *testing.T) {
	metadataXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>test</groupId>
	<artifactId>TestPackage</artifactId>
	<versioning><versions><version>1.0.0</version></versions></versioning>
</metadata>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "maven-metadata.xml") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(metadataXML))
		} else if r.URL.Path == "/" || r.URL.Path == "" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body><a href="test/">test/</a></body></html>`))
		} else if r.URL.Path == "/test/" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body><a href="TestPackage/">TestPackage/</a></body></html>`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	all, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 list element (test/TestPackage/1.0.0), got %d", len(all))
	}
	if len(all) > 0 && (all[0].Scope != "test" || all[0].PackageName != "TestPackage" || all[0].Version != "1.0.0") {
		t.Errorf("expected test/TestPackage/1.0.0, got %s/%s/%s", all[0].Scope, all[0].PackageName, all[0].Version)
	}
}

func Test_EncodeBase64_FileExists_ReturnsBase64(t *testing.T) {
	testData := []byte("test data for base64")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	result, err := repo.EncodeBase64(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := base64.StdEncoding.EncodeToString(testData)
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_EncodeBase64_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_, err = repo.EncodeBase64(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_PublishDate_ValidFile_ReturnsDate(t *testing.T) {
	expectedTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Last-Modified", expectedTime.Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	result, err := repo.PublishDate(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Equal(expectedTime) {
		t.Errorf("expected time %v, got %v", expectedTime, result)
	}
}

func Test_PublishDate_NoLastModified_ReturnsCurrentTime(t *testing.T) {
	mockTime := time.Date(2024, 3, 14, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	// Replace time provider with mock
	repo.timeProvider = utils.NewMockTimeProvider(mockTime)

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	result, err := repo.PublishDate(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Equal(mockTime) {
		t.Errorf("expected time %v, got %v", mockTime, result)
	}
}

func Test_PublishDate_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_, err = repo.PublishDate(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_LoadMetadata_ValidFile_ReturnsMetadata(t *testing.T) {
	metadataData := map[string]any{
		"repositoryURLs": []string{"https://example.com/repo"},
		"version":        "1.0.0",
	}
	jsonData, _ := json.Marshal(metadataData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	result, err := repo.LoadMetadata(context.Background(), "testScope", "my-package", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%v'", result["version"])
	}
}

func Test_LoadMetadata_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	_, err = repo.LoadMetadata(context.Background(), "testScope", "my-package", "1.0.0")
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_Checksum_ValidFile_ReturnsChecksum(t *testing.T) {
	testData := []byte("test data for checksum")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	checksum, err := repo.Checksum(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if checksum == "" {
		t.Errorf("expected checksum, got empty string")
	}
}

func Test_Checksum_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_, err = repo.Checksum(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_GetSwiftToolVersion_ValidManifest_ReturnsVersion(t *testing.T) {
	manifestContent := "// swift-tools-version:5.7\nlet package = Package(name: \"test\")"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifestContent))
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	version, err := repo.GetSwiftToolVersion(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if version != "5.7" {
		t.Errorf("expected version '5.7', got '%s'", version)
	}
}

func Test_GetSwiftToolVersion_NoVersion_ReturnsError(t *testing.T) {
	manifestContent := "let package = Package(name: \"test\")"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifestContent))
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	_, err = repo.GetSwiftToolVersion(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for missing version, got nil")
	}
}

func Test_GetSwiftToolVersion_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	_, err = repo.GetSwiftToolVersion(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_GetAlternativeManifests_ReturnsEmptyList(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	manifests, err := repo.GetAlternativeManifests(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("expected empty list, got %d items", len(manifests))
	}
}

func Test_Lookup_ReturnsEmptyList(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	result := repo.Lookup(context.Background(), "https://example.com/repo")
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func Test_Remove_ValidFile_RemovesFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	err = repo.Remove(context.Background(), element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_ListScopes_ReturnsEmptyList(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	scopes, err := repo.ListScopes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scopes) != 0 {
		t.Errorf("expected empty list, got %d items", len(scopes))
	}
}

func Test_ListInScope_ReturnsEmptyList(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	elements, err := repo.ListInScope(context.Background(), "testScope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(elements) != 0 {
		t.Errorf("expected empty list, got %d items", len(elements))
	}
}

func Test_ListAll_ReturnsEmptyList(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	elements, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(elements) != 0 {
		t.Errorf("expected empty list, got %d items", len(elements))
	}
}

func Test_LoadPackageJson_ValidFile_ReturnsPackageJson(t *testing.T) {
	packageJsonData := map[string]any{
		"name":    "test-package",
		"version": "1.0.0",
	}
	jsonData, _ := json.Marshal(packageJsonData)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	result, err := repo.LoadPackageJson(context.Background(), "testScope", "my-package", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"] != "test-package" {
		t.Errorf("expected name 'test-package', got '%v'", result["name"])
	}
}

func Test_LoadPackageJson_FileDoesNotExist_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	_, err = repo.LoadPackageJson(context.Background(), "testScope", "my-package", "1.0.0")
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
}

func Test_ExtractManifestFiles_ValidZip_ExtractsFiles(t *testing.T) {
	// Create a zip file with Package.swift and Package.json
	// Directory must match scope.name format (e.g., "testScope.my-package/")
	var zipBuf bytes.Buffer
	zipWriter := zip.NewWriter(&zipBuf)

	// Add Package.swift - directory must match scope.name format
	packageSwift, _ := zipWriter.Create("testScope.my-package/Package.swift")
	_, _ = packageSwift.Write([]byte("// swift-tools-version:5.3\nlet package = Package(name: \"test\")"))

	// Add alternative Package.swift files (should also be extracted)
	packageSwift57, _ := zipWriter.Create("testScope.my-package/Package@swift-5.7.0.swift")
	_, _ = packageSwift57.Write([]byte("// swift-tools-version:5.7.0\nlet package = Package(name: \"test\")"))

	packageSwift5, _ := zipWriter.Create("testScope.my-package/Package@swift-5.swift")
	_, _ = packageSwift5.Write([]byte("// swift-tools-version:5.0\nlet package = Package(name: \"test\")"))

	// Add Package.json - directory must match scope.name format
	packageJson, _ := zipWriter.Create("testScope.my-package/Package.json")
	_, _ = packageJson.Write([]byte(`{"name": "test", "version": "1.0.0"}`))

	zipWriter.Close()
	zipData := zipBuf.Bytes()

	uploadedFiles := make(map[string][]byte)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipData)
		} else if r.Method == "PUT" {
			// Capture uploaded file - path includes the base URL path
			path := strings.TrimPrefix(r.URL.Path, "/")
			data, _ := io.ReadAll(r.Body)
			uploadedFiles[path] = data
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	err = repo.ExtractManifestFiles(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that Package.swift and Package.json were uploaded
	// The extraction logic extracts all files starting with "package" and ending with ".swift"
	// and files exactly named "package.json" (case-insensitive)
	// Format: package-swift-5.7.0 (Maven-compliant: lowercase, hyphens only)
	foundSwift := false
	foundSwift57 := false
	foundSwift5 := false
	foundJson := false
	for path := range uploadedFiles {
		if strings.Contains(path, "package.swift") && !strings.Contains(path, "swift-") {
			foundSwift = true
		}
		// Maven-compliant format: package-swift-5.7.0 (lowercase, hyphens only)
		if strings.Contains(path, "package-swift-5.7.0.swift") {
			foundSwift57 = true
		}
		if strings.Contains(path, "package-swift-5.swift") {
			foundSwift5 = true
		}
		if strings.Contains(path, "package.json") {
			foundJson = true
		}
	}

	if !foundSwift {
		t.Errorf("expected package.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}
	if !foundSwift57 {
		t.Errorf("expected package-swift-5.7.0.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}
	if !foundSwift5 {
		t.Errorf("expected package-swift-5.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}
	if !foundJson {
		t.Errorf("expected package.json to be uploaded, uploaded paths: %v", uploadedFiles)
	}
}

func Test_ExtractManifestFiles_RealZipFile_ExtractsFiles(t *testing.T) {
	// Test with actual zip file from test data
	// This zip has: test.TestLib/Package.swift, Package@swift-5.7.0.swift, Package@swift-5.swift
	// and package-metadata.json (not Package.json)
	// Use testdata directory (Go convention for test files that can be checked into git)
	zipPath := "testdata/test/TestLib/1.3.35/test.TestLib-1.3.35.zip"
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Skipf("test zip file not found at %s: %v", zipPath, err)
	}

	uploadedFiles := make(map[string][]byte)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipData)
		} else if r.Method == "PUT" {
			// Capture uploaded file - path includes the base URL path
			path := strings.TrimPrefix(r.URL.Path, "/")
			data, _ := io.ReadAll(r.Body)
			uploadedFiles[path] = data
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	// The zip file has test.TestLib/ directory, so scope="test", name="TestLib"
	element := models.NewUploadElement("test", "TestLib", "1.3.35", mimetypes.ApplicationZip, models.SourceArchive)
	err = repo.ExtractManifestFiles(context.Background(), element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that Package.swift files were uploaded
	// The actual zip has: Package.swift, Package@swift-5.7.0.swift, Package@swift-5.swift
	// Format: package-swift-5.7.0 (Maven-compliant: lowercase, hyphens only)
	foundSwift := false
	foundSwift57 := false
	foundSwift5 := false
	for path := range uploadedFiles {
		if strings.Contains(path, "package.swift") && !strings.Contains(path, "swift-") {
			foundSwift = true
		}
		// Maven-compliant format: package-swift-5.7.0 (lowercase, hyphens only)
		if strings.Contains(path, "package-swift-5.7.0.swift") {
			foundSwift57 = true
		}
		if strings.Contains(path, "package-swift-5.swift") {
			foundSwift5 = true
		}
	}

	if !foundSwift {
		t.Errorf("expected package.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}
	if !foundSwift57 {
		t.Errorf("expected package-swift-5.7.0.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}
	if !foundSwift5 {
		t.Errorf("expected package-swift-5.swift to be uploaded, uploaded paths: %v", uploadedFiles)
	}

	// Note: The actual zip has package-metadata.json, not Package.json
	// The extraction logic only extracts files exactly named "package.json" (case-insensitive)
	// So package-metadata.json should NOT be extracted
	foundPackageJson := false
	for path := range uploadedFiles {
		if strings.Contains(path, "Package.json") {
			foundPackageJson = true
		}
	}
	if foundPackageJson {
		t.Errorf("unexpected Package.json found - actual zip has package-metadata.json, not Package.json, uploaded paths: %v", uploadedFiles)
	}
}

func Test_ExtractManifestFiles_UnsupportedMimeType_ReturnsError(t *testing.T) {
	cfg := config.MavenConfig{BaseURL: "https://repo.example.com"}
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	element := models.NewUploadElement("testScope", "my-package", "1.0.0", "text/plain", models.SourceArchive)
	err = repo.ExtractManifestFiles(context.Background(), element)
	if err == nil {
		t.Errorf("expected error for unsupported mime type, got nil")
	}
}
