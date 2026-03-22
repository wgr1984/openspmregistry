package controller

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"OpenSPMRegistry/collectionsign"
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/responses"
)

type collectionTestRepo struct {
	MockRepo
	listAll        []models.ListElement
	listInScope    []models.ListElement
	listAllErr     error
	listInScopeErr error
	packageJson    map[string]any
	metadata       map[string]any
	swiftVersion   string
	publishDate    time.Time
}

func Test_GlobalCollectionAction_CollectionsDisabled_ReturnsNotFound(t *testing.T) {
	c := &Controller{
		config: config.ServerConfig{
			PackageCollections: config.PackageCollectionsConfig{Enabled: false},
		},
	}
	req := httptest.NewRequest("GET", "/collection", nil)
	w := httptest.NewRecorder()

	c.GlobalCollectionAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
	var resp responses.Error
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Detail != "Package collections are not enabled" {
		t.Fatalf("unexpected error detail: %s", resp.Detail)
	}
}

func Test_GlobalCollectionAction_WithCollectionSigner_ReturnsSignedJSON(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestP256CertForCollection(t, dir)
	signer, err := collectionsign.NewSignerFromFiles([]string{certPath}, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	repo := newCollectionTestRepo([]models.ListElement{
		{Scope: "scope", PackageName: "pkg", Version: "1.0.0"},
	})
	c := NewController(
		config.ServerConfig{
			Hostname:           "example.com",
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo,
		WithCollectionSigner(signer),
	)

	req := httptest.NewRequest("GET", "/collection", nil)
	w := httptest.NewRecorder()
	c.GlobalCollectionAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &root); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := root["signature"]; !ok {
		t.Fatal("expected signed collection to include signature key")
	}
	if _, ok := root["packages"]; !ok {
		t.Fatal("expected packages key")
	}
}

func Test_GlobalCollectionAction_ReturnsCollectionJSON(t *testing.T) {
	repo := newCollectionTestRepo([]models.ListElement{
		{Scope: "scope", PackageName: "pkg", Version: "1.0.0"},
	})
	c := &Controller{
		config: config.ServerConfig{
			Hostname:           "example.com",
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo: repo,
	}

	req := httptest.NewRequest("GET", "/collection", nil)
	w := httptest.NewRecorder()

	c.GlobalCollectionAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected content type application/json, got %s", contentType)
	}

	var coll models.PackageCollection
	if err := json.NewDecoder(w.Body).Decode(&coll); err != nil {
		t.Fatalf("failed to decode collection: %v", err)
	}

	if coll.Name != "example.com: All Packages" {
		t.Fatalf("expected collection name %q, got %q", "example.com: All Packages", coll.Name)
	}
	if len(coll.Packages) != 1 {
		t.Fatalf("expected exactly one package, got %d", len(coll.Packages))
	}
	if coll.Packages[0].URL != "scope.pkg" {
		t.Fatalf("unexpected package url %q", coll.Packages[0].URL)
	}
}

func Test_ScopeCollectionAction_MissingScope_ReturnsBadRequest(t *testing.T) {
	c := &Controller{
		config: config.ServerConfig{
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo: newCollectionTestRepo(nil),
	}
	req := httptest.NewRequest("GET", "/collection/", nil)
	w := httptest.NewRecorder()

	c.ScopeCollectionAction(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	var resp responses.Error
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Detail != "Scope is required" {
		t.Fatalf("unexpected error detail: %s", resp.Detail)
	}
}

func Test_ScopeCollectionAction_ListError_ReturnsNotFound(t *testing.T) {
	repo := newCollectionTestRepo([]models.ListElement{
		{Scope: "scope", PackageName: "pkg", Version: "1.0.0"},
	})
	repo.listInScopeErr = errors.New("boom")
	c := &Controller{
		config: config.ServerConfig{
			Hostname:           "example.com",
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo: repo,
	}

	req := httptest.NewRequest("GET", "/collection/scope", nil)
	req.SetPathValue("scope", "scope")
	w := httptest.NewRecorder()

	c.ScopeCollectionAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
	var resp responses.Error
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Detail != "Scope not found" {
		t.Fatalf("unexpected error detail: %s", resp.Detail)
	}
}

func Test_ScopeCollectionAction_EmptyScope_ReturnsNotFound(t *testing.T) {
	repo := newCollectionTestRepo([]models.ListElement{
		{Scope: "scope", PackageName: "pkg", Version: "1.0.0"},
	})
	repo.listInScope = []models.ListElement{}
	c := &Controller{
		config: config.ServerConfig{
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo: repo,
	}

	req := httptest.NewRequest("GET", "/collection/scope", nil)
	req.SetPathValue("scope", "scope")
	w := httptest.NewRecorder()

	c.ScopeCollectionAction(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
	var resp responses.Error
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Detail != "No packages found in scope" {
		t.Fatalf("unexpected error detail: %s", resp.Detail)
	}
}

func Test_ScopeCollectionAction_ReturnsScopeCollection(t *testing.T) {
	repo := newCollectionTestRepo([]models.ListElement{
		{Scope: "scope", PackageName: "pkg", Version: "1.0.0"},
	})
	c := &Controller{
		config: config.ServerConfig{
			Hostname:           "example.com",
			PackageCollections: config.PackageCollectionsConfig{Enabled: true},
		},
		repo: repo,
	}

	req := httptest.NewRequest("GET", "/collection/scope", nil)
	req.SetPathValue("scope", "scope")
	w := httptest.NewRecorder()

	c.ScopeCollectionAction(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	var coll models.PackageCollection
	if err := json.NewDecoder(w.Body).Decode(&coll); err != nil {
		t.Fatalf("failed to decode collection: %v", err)
	}
	if coll.Name != "example.com: scope Packages" {
		t.Fatalf("unexpected collection name %q", coll.Name)
	}
	if coll.Overview != "Package collection for scope scope" {
		t.Fatalf("unexpected overview %q", coll.Overview)
	}
	if len(coll.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(coll.Packages))
	}
}

func newCollectionTestRepo(elements []models.ListElement) *collectionTestRepo {
	pkgName := "pkg"
	if len(elements) > 0 {
		pkgName = elements[0].PackageName
	}
	return &collectionTestRepo{
		listAll:     elements,
		listInScope: elements,
		packageJson: map[string]any{
			"name": pkgName,
			"targets": []any{
				map[string]any{"name": "Target"},
			},
			"products": []any{
				map[string]any{
					"name":    "Product",
					"targets": []any{"Target"},
					"type": map[string]any{
						"library": []any{"automatic"},
					},
				},
			},
			"platforms": []any{
				map[string]any{
					"platformName": "macos",
					"version":      "10.15",
				},
			},
		},
		metadata: map[string]any{
			"description": "test package",
			"licenseURL":  "https://example.com/license",
			"readmeURL":   "https://example.com/readme",
		},
		swiftVersion: "5.10.0",
		publishDate:  time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (r *collectionTestRepo) ListAll(ctx context.Context) ([]models.ListElement, error) {
	if r.listAllErr != nil {
		return nil, r.listAllErr
	}
	return r.listAll, nil
}

func (r *collectionTestRepo) ListInScope(ctx context.Context, scope string) ([]models.ListElement, error) {
	if r.listInScopeErr != nil {
		return nil, r.listInScopeErr
	}
	if r.listInScope != nil {
		return r.listInScope, nil
	}
	var filtered []models.ListElement
	for _, elem := range r.listAll {
		if elem.Scope == scope {
			filtered = append(filtered, elem)
		}
	}
	return filtered, nil
}

func (r *collectionTestRepo) LoadPackageJson(ctx context.Context, scope string, name string, version string) (map[string]any, error) {
	return r.packageJson, nil
}

func (r *collectionTestRepo) LoadMetadata(ctx context.Context, scope string, name string, version string) (map[string]any, error) {
	return r.metadata, nil
}

func (r *collectionTestRepo) GetSwiftToolVersion(ctx context.Context, manifest *models.UploadElement) (string, error) {
	if r.swiftVersion == "" {
		return "5.10", nil
	}
	return r.swiftVersion, nil
}

func (r *collectionTestRepo) PublishDate(ctx context.Context, element *models.UploadElement) (time.Time, error) {
	return r.publishDate, nil
}

func writeTestP256CertForCollection(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "collection-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(dir, "leaf.der")
	if err := os.WriteFile(certPath, der, 0o600); err != nil {
		t.Fatal(err)
	}
	pk8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
