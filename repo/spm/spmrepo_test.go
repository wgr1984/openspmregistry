package spm

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── helpers ──────────────────────────────────────────────────────────────

func newTestRepo(t *testing.T, handler http.HandlerFunc) (*SpmRepo, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	cfg := config.SPMConfig{BaseURL: srv.URL}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		srv.Close()
		t.Fatalf("NewSpmRepo: %v", err)
	}
	return repo, srv
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ─── constructor ──────────────────────────────────────────────────────────

func Test_NewSpmRepo_ValidConfig(t *testing.T) {
	cfg := config.SPMConfig{BaseURL: "https://registry.example.com"}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo")
	}
}

func Test_NewSpmRepo_MissingBaseURL_ReturnsError(t *testing.T) {
	_, err := NewSpmRepo(config.SPMConfig{})
	if err == nil {
		t.Fatal("expected error for missing BaseURL")
	}
}

func Test_NewSpmRepo_WithLocalPath_EnablesSplitMode(t *testing.T) {
	dir := t.TempDir()
	cfg := config.SPMConfig{BaseURL: "https://registry.example.com", LocalPath: dir}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.isSplitMode() {
		t.Error("expected split mode to be enabled")
	}
}

// ─── buildUpstreamPath ────────────────────────────────────────────────────

func Test_buildUpstreamPath_SourceArchive(t *testing.T) {
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	path, query := buildUpstreamPath(el)
	if path != "/scope/pkg/1.0.0.zip" {
		t.Errorf("unexpected path: %s", path)
	}
	if query != "" {
		t.Errorf("unexpected query: %s", query)
	}
}

func Test_buildUpstreamPath_Manifest(t *testing.T) {
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	path, query := buildUpstreamPath(el)
	if path != "/scope/pkg/1.0.0/Package.swift" {
		t.Errorf("unexpected path: %s", path)
	}
	if query != "" {
		t.Errorf("unexpected query: %s", query)
	}
}

func Test_buildUpstreamPath_ManifestVariant(t *testing.T) {
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	el.SetFilenameOverwrite("Package@swift-5.8")
	path, query := buildUpstreamPath(el)
	if path != "/scope/pkg/1.0.0/Package.swift" {
		t.Errorf("unexpected path: %s", path)
	}
	if query != "swift-version=5.8" {
		t.Errorf("unexpected query: %s", query)
	}
}

func Test_buildUpstreamPath_Signature_ReturnsEmpty(t *testing.T) {
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchiveSignature)
	path, query := buildUpstreamPath(el)
	if path != "" || query != "" {
		t.Errorf("expected empty path/query for signature, got path=%q query=%q", path, query)
	}
}

func Test_buildUpstreamPath_PackageJson_ReturnsEmpty(t *testing.T) {
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationJson, models.PackageManifestJson)
	path, query := buildUpstreamPath(el)
	if path != "" || query != "" {
		t.Errorf("expected empty path/query for Package.json, got path=%q query=%q", path, query)
	}
}

// ─── Exists ───────────────────────────────────────────────────────────────

func Test_Exists_SourceArchive_Found(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/scope/pkg/1.0.0.zip" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	if !repo.Exists(context.Background(), el) {
		t.Error("expected Exists to return true")
	}
}

func Test_Exists_SourceArchive_NotFound(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	if repo.Exists(context.Background(), el) {
		t.Error("expected Exists to return false")
	}
}

func Test_Exists_Signature_ProxyMode_ReturnsFalse(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // should not be called
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchiveSignature)
	if repo.Exists(context.Background(), el) {
		t.Error("expected Exists to return false for signature in proxy mode")
	}
}

// ─── GetReader ────────────────────────────────────────────────────────────

func Test_GetReader_SourceArchive_Success(t *testing.T) {
	content := []byte("zip content")
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/scope/pkg/1.0.0.zip" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	reader, err := repo.GetReader(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = reader.Close() }()
}

func Test_GetReader_Signature_ProxyMode_ReturnsError(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchiveSignature)
	_, err := repo.GetReader(context.Background(), el)
	if err == nil {
		t.Error("expected error for signature in proxy mode")
	}
}

// ─── GetWriter ────────────────────────────────────────────────────────────

func Test_GetWriter_ProxyMode_ReturnsError(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_, err := repo.GetWriter(context.Background(), el)
	if err == nil {
		t.Error("expected error for GetWriter in proxy mode")
	}
}

func Test_GetWriter_SplitMode_SignatureStored(t *testing.T) {
	dir := t.TempDir()
	cfg := config.SPMConfig{
		BaseURL:       "https://registry.example.com",
		LocalPath:     dir,
		StoreSignings: true,
	}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		t.Fatalf("NewSpmRepo: %v", err)
	}

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchiveSignature)
	writer, err := repo.GetWriter(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, _ = writer.Write([]byte("sig data"))
	_ = writer.Close()
}

// ─── List ─────────────────────────────────────────────────────────────────

func Test_List_ReturnsVersions(t *testing.T) {
	releases := map[string]any{
		"releases": map[string]any{
			"1.0.0": map[string]any{"url": "https://example.com/scope/pkg/1.0.0"},
			"2.0.0": map[string]any{"url": "https://example.com/scope/pkg/2.0.0"},
		},
	}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scope/pkg" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(releases))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	elements, err := repo.List(context.Background(), "scope", "pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elements) != 2 {
		t.Errorf("expected 2 versions, got %d", len(elements))
	}
}

func Test_List_UpstreamError_ReturnsEmpty(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	elements, err := repo.List(context.Background(), "scope", "pkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elements) != 0 {
		t.Errorf("expected 0 versions, got %d", len(elements))
	}
}

// ─── Checksum ─────────────────────────────────────────────────────────────

func Test_Checksum_ReturnsChecksumFromVersionInfo(t *testing.T) {
	info := map[string]any{
		"resources": []any{
			map[string]any{
				"name":     "source-archive",
				"type":     "application/zip",
				"checksum": "abc123",
			},
		},
	}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scope/pkg/1.0.0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(info))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	checksum, err := repo.Checksum(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checksum != "abc123" {
		t.Errorf("expected checksum 'abc123', got %q", checksum)
	}
}

func Test_Checksum_NoResource_ReturnsError(t *testing.T) {
	info := map[string]any{
		"resources": []any{},
	}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(mustMarshal(info))
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_, err := repo.Checksum(context.Background(), el)
	if err == nil {
		t.Error("expected error when source-archive resource is absent")
	}
}

// ─── LoadMetadata ─────────────────────────────────────────────────────────

func Test_LoadMetadata_ReturnsMetadataFromVersionInfo(t *testing.T) {
	info := map[string]any{
		"metadata": map[string]any{
			"author": "Alice",
		},
	}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scope/pkg/1.0.0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(info))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	metadata, err := repo.LoadMetadata(context.Background(), "scope", "pkg", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metadata["author"] != "Alice" {
		t.Errorf("unexpected metadata: %v", metadata)
	}
}

func Test_LoadMetadata_NoMetadataField_ReturnsError(t *testing.T) {
	info := map[string]any{}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(mustMarshal(info))
	})
	defer srv.Close()

	_, err := repo.LoadMetadata(context.Background(), "scope", "pkg", "1.0.0")
	if err == nil {
		t.Error("expected error when metadata field is absent")
	}
}

// ─── PublishDate ──────────────────────────────────────────────────────────

func Test_PublishDate_LastModifiedHeader(t *testing.T) {
	modTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Last-Modified", modTime.Format(time.RFC1123))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	got, err := repo.PublishDate(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(modTime) {
		t.Errorf("expected %v, got %v", modTime, got)
	}
}

func Test_PublishDate_FallsBackToVersionInfoPublishedAt(t *testing.T) {
	publishedAt := "2024-03-01T10:00:00Z"
	info := map[string]any{"publishedAt": publishedAt}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK) // no Last-Modified
			return
		}
		_, _ = w.Write(mustMarshal(info))
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	got, err := repo.PublishDate(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := time.Parse(time.RFC3339, publishedAt)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// ─── GetAlternativeManifests ──────────────────────────────────────────────

func Test_GetAlternativeManifests_ParsesLinkHeader(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/Package.swift") {
			w.Header().Set("Link", `<https://example.com/scope/pkg/1.0.0/Package.swift?swift-version=5.8>; rel="alternate"; filename="Package@swift-5.8.swift"`)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	manifests, err := repo.GetAlternativeManifests(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 alternate manifest, got %d", len(manifests))
	}
	if manifests[0].FileName() != "Package@swift-5.8.swift" {
		t.Errorf("unexpected filename: %s", manifests[0].FileName())
	}
}

func Test_GetAlternativeManifests_NoLinkHeader_ReturnsEmpty(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	manifests, err := repo.GetAlternativeManifests(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 alternate manifests, got %d", len(manifests))
	}
}

// ─── parseLinkAlternateFilenames ──────────────────────────────────────────

func Test_parseLinkAlternateFilenames_SingleEntry(t *testing.T) {
	header := `<https://example.com/Package.swift?swift-version=5.8>; rel="alternate"; filename="Package@swift-5.8.swift"`
	filenames := parseLinkAlternateFilenames(header)
	if len(filenames) != 1 || filenames[0] != "Package@swift-5.8.swift" {
		t.Errorf("unexpected filenames: %v", filenames)
	}
}

func Test_parseLinkAlternateFilenames_MultipleEntries(t *testing.T) {
	header := `<https://example.com/Package.swift?swift-version=5.8>; rel="alternate"; filename="Package@swift-5.8.swift", ` +
		`<https://example.com/Package.swift?swift-version=5.9>; rel="alternate"; filename="Package@swift-5.9.swift"`
	filenames := parseLinkAlternateFilenames(header)
	if len(filenames) != 2 {
		t.Errorf("expected 2 filenames, got %d: %v", len(filenames), filenames)
	}
}

func Test_parseLinkAlternateFilenames_NonAlternate_Ignored(t *testing.T) {
	header := `<https://example.com/Package.swift>; rel="canonical"`
	filenames := parseLinkAlternateFilenames(header)
	if len(filenames) != 0 {
		t.Errorf("expected 0 filenames, got %d", len(filenames))
	}
}

// ─── Lookup ───────────────────────────────────────────────────────────────

func Test_Lookup_ReturnsIdentifiers(t *testing.T) {
	response := map[string]any{
		"identifiers": []string{"scope.pkg"},
	}
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identifiers" && r.URL.Query().Get("url") == "https://github.com/example/pkg" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(mustMarshal(response))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	ids := repo.Lookup(context.Background(), "https://github.com/example/pkg")
	if len(ids) != 1 || ids[0] != "scope.pkg" {
		t.Errorf("unexpected identifiers: %v", ids)
	}
}

func Test_Lookup_UpstreamError_ReturnsNil(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	ids := repo.Lookup(context.Background(), "https://github.com/example/pkg")
	if ids != nil {
		t.Errorf("expected nil, got %v", ids)
	}
}

// ─── ListScopes / ListInScope / ListAll ───────────────────────────────────

func Test_ListScopes_ProxyMode_ReturnsEmpty(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	scopes, err := repo.ListScopes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 0 {
		t.Errorf("expected 0 scopes, got %d", len(scopes))
	}
}

func Test_ListInScope_ProxyMode_ReturnsEmpty(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	elements, err := repo.ListInScope(context.Background(), "scope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(elements))
	}
}

func Test_ListAll_ProxyMode_ReturnsEmpty(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	elements, err := repo.ListAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(elements))
	}
}

// ─── LoadPackageJson ──────────────────────────────────────────────────────

func Test_LoadPackageJson_ProxyMode_ReturnsError(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	_, err := repo.LoadPackageJson(context.Background(), "scope", "pkg", "1.0.0")
	if err == nil {
		t.Error("expected error in proxy mode")
	}
}

// ─── GetSwiftToolVersion ──────────────────────────────────────────────────

func Test_GetSwiftToolVersion_ReturnsVersion(t *testing.T) {
	manifestContent := "// swift-tools-version: 5.8\nimport PackageDescription"
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scope/pkg/1.0.0/Package.swift" {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, manifestContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	version, err := repo.GetSwiftToolVersion(context.Background(), el)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "5.8" {
		t.Errorf("expected '5.8', got %q", version)
	}
}

func Test_GetSwiftToolVersion_NoPrefix_ReturnsError(t *testing.T) {
	manifestContent := "import PackageDescription"
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, manifestContent)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.TextXSwift, models.Manifest)
	_, err := repo.GetSwiftToolVersion(context.Background(), el)
	if err == nil {
		t.Error("expected error when swift-tools-version prefix is absent")
	}
}

// ─── Remove ───────────────────────────────────────────────────────────────

func Test_Remove_ProxyMode_ReturnsNil(t *testing.T) {
	repo, srv := newTestRepo(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	if err := repo.Remove(context.Background(), el); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ─── auth ─────────────────────────────────────────────────────────────────

func Test_Auth_Passthrough_ForwardsHeader(t *testing.T) {
	receivedAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.SPMConfig{BaseURL: srv.URL, AuthMode: "passthrough"}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		t.Fatalf("NewSpmRepo: %v", err)
	}
	ctx := context.WithValue(context.Background(), config.AuthHeaderContextKey, "Bearer token123")
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_ = repo.Exists(ctx, el)

	if receivedAuth != "Bearer token123" {
		t.Errorf("expected forwarded auth header, got %q", receivedAuth)
	}
}

func Test_Auth_Config_SendsConfiguredCredentials(t *testing.T) {
	receivedAuth := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.SPMConfig{
		BaseURL:  srv.URL,
		AuthMode: "config",
		Username: "user",
		Password: "pass",
	}
	repo, err := NewSpmRepo(cfg)
	if err != nil {
		t.Fatalf("NewSpmRepo: %v", err)
	}
	el := models.NewUploadElement("scope", "pkg", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	_ = repo.Exists(context.Background(), el)

	if !strings.HasPrefix(receivedAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got %q", receivedAuth)
	}
}

// ─── isSigningElement / isIndexElement ────────────────────────────────────

func Test_isSigningElement_SourceArchiveSignature(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationZip, models.SourceArchiveSignature)
	if !isSigningElement(el) {
		t.Error("expected true for SourceArchiveSignature")
	}
}

func Test_isSigningElement_MetadataSignature(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationJson, models.MetadataSignature)
	if !isSigningElement(el) {
		t.Error("expected true for MetadataSignature")
	}
}

func Test_isSigningElement_SourceArchive_False(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	if isSigningElement(el) {
		t.Error("expected false for SourceArchive")
	}
}

func Test_isIndexElement_PackageJson(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationJson, models.PackageManifestJson)
	if !isIndexElement(el) {
		t.Error("expected true for PackageManifestJson")
	}
}

func Test_isIndexElement_Metadata(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationJson, models.Metadata)
	if !isIndexElement(el) {
		t.Error("expected true for Metadata")
	}
}

func Test_isIndexElement_SourceArchive_False(t *testing.T) {
	el := models.NewUploadElement("s", "p", "1.0.0", mimetypes.ApplicationZip, models.SourceArchive)
	if isIndexElement(el) {
		t.Error("expected false for SourceArchive")
	}
}
