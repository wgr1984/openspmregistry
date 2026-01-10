package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/responses"
)

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

func (r *collectionTestRepo) ListAll() ([]models.ListElement, error) {
	if r.listAllErr != nil {
		return nil, r.listAllErr
	}
	return r.listAll, nil
}

func (r *collectionTestRepo) ListInScope(scope string) ([]models.ListElement, error) {
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

func (r *collectionTestRepo) LoadPackageJson(scope string, name string, version string) (map[string]any, error) {
	return r.packageJson, nil
}

func (r *collectionTestRepo) LoadMetadata(scope string, name string, version string) (map[string]any, error) {
	return r.metadata, nil
}

func (r *collectionTestRepo) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	if r.swiftVersion == "" {
		return "5.10", nil
	}
	return r.swiftVersion, nil
}

func (r *collectionTestRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	return r.publishDate, nil
}
