//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the SPM proxy backend.
//
// Run with: go test -tags=e2e -v ./e2e/... -run TestRegistryE2ESPM
// No external services are required: the upstream is a mock httptest.Server started in-process.
// Set E2E_TESTS=1 to enable.
package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	spmE2EScope      = "e2espm"
	spmTestPackage   = "SpmPkg"
	spmVersion1      = "1.0.0"
	spmSignedPackage = "SpmSignedPkg"
	spmSignedVersion = "1.0.0"
)

// writeSPMProxyConfig writes a registry config file for the SPM proxy backend to a temp
// directory and returns the file path.  upstreamURL is the base URL of the upstream SPM
// registry. localPath, storeSignings and storeIndex configure split mode; split mode is
// disabled when localPath is empty.
func writeSPMProxyConfig(t *testing.T, upstreamURL, localPath string, storeSignings, storeIndex bool) string {
	t.Helper()
	splitSection := ""
	if localPath != "" {
		splitSection = fmt.Sprintf(
			"\n      localPath: %s\n      storeSignings: %v\n      storeIndex: %v",
			localPath, storeSignings, storeIndex,
		)
	}
	content := fmt.Sprintf(`server:
  hostname: 127.0.0.1
  port: 8082
  listPageSize: 0
  tlsEnabled: false
  repo:
    type: spm
    spm:
      baseURL: %s
      timeout: 10%s
  publish:
    maxSize: 204800
  auth:
    enabled: false
  packageCollections:
    enabled: true
    requirePackageJson: false
`, upstreamURL, splitSection)
	configPath := filepath.Join(t.TempDir(), "config-spm.yml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write spm proxy config: %v", err)
	}
	return configPath
}

// mockSPMUpstreamHandler returns an http.Handler that mimics an upstream Swift Package Registry
// for a single package identified by scope/pkg/version.
//
// It handles:
//   - HEAD  /{scope}/{pkg}/{version}.zip             — Last-Modified header
//   - GET   /{scope}/{pkg}/{version}.zip             — source archive download
//   - HEAD  /{scope}/{pkg}/{version}/Package.swift   — Link header with variant alternate
//   - GET   /{scope}/{pkg}/{version}/Package.swift   — manifest (default or swift-version query)
//   - GET   /{scope}/{pkg}/{version}                 — version info JSON
//   - GET   /{scope}/{pkg}                           — releases list JSON
//   - GET   /identifiers?url=https://github.com/example/{pkg}  — identifier lookup
//   - everything else returns 404
func mockSPMUpstreamHandler(
	scope, pkg, version string,
	zipData []byte,
	checksumHex string,
	packageSwiftContent string,
	variantSwiftContent string,
) http.Handler {
	archivePath := fmt.Sprintf("/%s/%s/%s.zip", scope, pkg, version)
	pkgSwiftPath := fmt.Sprintf("/%s/%s/%s/Package.swift", scope, pkg, version)
	versionInfoPath := fmt.Sprintf("/%s/%s/%s", scope, pkg, version)
	releasesPath := fmt.Sprintf("/%s/%s", scope, pkg)
	identifiersPath := "/identifiers"
	lastModified := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC1123)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimRight(r.URL.Path, "/")
		swiftVersion := r.URL.Query().Get("swift-version")

		switch {
		// Source archive HEAD — expose Last-Modified for PublishDate
		case path == archivePath && r.Method == http.MethodHead:
			w.Header().Set("Last-Modified", lastModified)
			w.WriteHeader(http.StatusOK)

		// Source archive GET — stream bytes
		case path == archivePath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(zipData)

		// Package.swift HEAD — include Link with variant so GetAlternativeManifests has data
		case path == pkgSwiftPath && r.Method == http.MethodHead:
			w.Header().Set("Last-Modified", lastModified)
			if variantSwiftContent != "" {
				w.Header().Set("Link", fmt.Sprintf(
					`<%s?swift-version=5.7.0>; rel="alternate"; filename="Package@swift-5.7.0.swift"`,
					pkgSwiftPath,
				))
			}
			w.WriteHeader(http.StatusOK)

		// Package.swift GET — serve default, variant, or 404 for unknown versions.
		// The proxy's HTTP client follows redirects, so returning 303 here would cause
		// the proxy to silently serve the default manifest instead of a 303/404.
		// Returning 404 ensures the proxy propagates it as a 404 to the client, which
		// is the expected behavior per spec 4.3.1.
		case path == pkgSwiftPath && r.Method == http.MethodGet:
			switch swiftVersion {
			case "5.7.0":
				if variantSwiftContent == "" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "text/x-swift")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(variantSwiftContent))
			case "":
				w.Header().Set("Content-Type", "text/x-swift")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(packageSwiftContent))
			default:
				// Unknown swift-version: no variant found → 404 so proxy propagates it
				w.WriteHeader(http.StatusNotFound)
			}

		// Version info GET
		case path == versionInfoPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      scope + "." + pkg,
				"version": version,
				"resources": []any{
					map[string]any{
						"name":     "source-archive",
						"type":     "application/zip",
						"checksum": checksumHex,
					},
				},
				"metadata": map[string]any{
					"description": "SPM proxy e2e test package for " + pkg,
				},
				"publishedAt": "2024-01-15T10:00:00Z",
			})

		// List releases GET
		case path == releasesPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"releases": map[string]any{
					version: map[string]any{
						"url": fmt.Sprintf("http://127.0.0.1:8082/%s/%s/%s", scope, pkg, version),
					},
				},
			})

		// Identifier lookup GET
		case path == identifiersPath && r.Method == http.MethodGet:
			if r.URL.Query().Get("url") == "https://github.com/example/"+pkg {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"identifiers": []string{scope + "." + pkg},
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// TestRegistryE2ESPMProxy tests the SPM proxy backend against a mock upstream SPM registry.
//
// It runs a real proxy registry server process pointing at an in-process httptest.Server that
// mimics an upstream Swift Package Registry, validating that the proxy correctly forwards all
// read operations to the upstream and returns Swift Package Registry spec-compliant responses.
//
// No external services (Maven, Nexus, etc.) are required.
// Run with: go test -tags=e2e -v ./e2e/... -run TestRegistryE2ESPMProxy
func TestRegistryE2ESPMProxy(t *testing.T) {
	env := setupE2E(t)
	// SPM proxy tests always use plain HTTP on the standard test port
	env.registryURL = "http://127.0.0.1:8082"
	env.useHTTPS = false
	env.httpClient = &http.Client{Timeout: 30 * time.Second}

	zip1 := createMinimalZip(t, spmE2EScope, spmTestPackage, spmVersion1, true /* withVariant */)
	h := sha256.Sum256(zip1)
	checksumHex := fmt.Sprintf("%x", h[:])

	packageSwift := fmt.Sprintf(
		"// swift-tools-version:6.0\nimport PackageDescription\nlet package = Package(name: %q, products: [.library(name: %q, targets: [%q])], targets: [.target(name: %q)])\n",
		spmTestPackage, spmTestPackage, spmTestPackage, spmTestPackage,
	)
	variantSwift := "// swift-tools-version:5.7.0\nimport PackageDescription\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])\n"

	upstream := httptest.NewServer(mockSPMUpstreamHandler(
		spmE2EScope, spmTestPackage, spmVersion1,
		zip1, checksumHex, packageSwift, variantSwift,
	))
	defer upstream.Close()

	env.configPath = writeSPMProxyConfig(t, upstream.URL, "", false, false)
	defer startRegistryServer(t, env)()
	time.Sleep(500 * time.Millisecond)

	t.Run("List", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte(`"`+spmVersion1+`"`)) {
			t.Fatalf("list missing version %s: %s", spmVersion1, string(body))
		}
	})

	t.Run("VersionInfo", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("version info: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		if info["id"] != spmE2EScope+"."+spmTestPackage {
			t.Fatalf("unexpected id: %v", info["id"])
		}
		if info["version"] != spmVersion1 {
			t.Fatalf("unexpected version: %v", info["version"])
		}
	})

	t.Run("Checksum", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("version info: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		resources, _ := info["resources"].([]any)
		if len(resources) == 0 {
			t.Fatal("no resources in version info")
		}
		r0, _ := resources[0].(map[string]any)
		checksum, _ := r0["checksum"].(string)
		if checksum == "" {
			t.Fatal("checksum empty")
		}
		if checksum != checksumHex {
			t.Fatalf("checksum: got %s, want %s", checksum, checksumHex)
		}
	})

	t.Run("PublishDate", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("version info: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		publishedAt, _ := info["publishedAt"].(string)
		if publishedAt == "" {
			t.Fatal("publishedAt empty")
		}
		if _, err := time.Parse(time.RFC3339, publishedAt); err != nil {
			t.Fatalf("parse publishedAt %q: %v", publishedAt, err)
		}
	})

	t.Run("Metadata", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("version info: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		meta, _ := info["metadata"].(map[string]any)
		if meta == nil {
			t.Fatal("metadata field missing or null")
		}
		if desc, _ := meta["description"].(string); desc == "" {
			t.Fatal("metadata.description empty")
		}
	})

	t.Run("DownloadSourceArchive", func(t *testing.T) {
		url := env.registryPath(spmE2EScope, spmTestPackage, spmVersion1) + ".zip"
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept", acceptZip)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("download: expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeZip {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeZip)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd == "" || !strings.Contains(cd, "attachment") {
			t.Fatalf("Content-Disposition: got %q", cd)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "public, immutable" {
			t.Fatalf("Cache-Control: got %q, want public, immutable", cc)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(body, zip1) {
			t.Fatal("download content does not match expected zip")
		}
	})

	t.Run("PackageSwift", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeSwift {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeSwift)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd == "" || !strings.Contains(cd, "attachment") || !strings.Contains(cd, "Package.swift") {
			t.Fatalf("Content-Disposition: got %q, want attachment with Package.swift", cd)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:6.0")) {
			t.Fatalf("manifest missing swift-tools-version:6.0: %s", string(body))
		}
	})

	// Spec 4.3: when multiple manifests exist the Link header MUST include alternate relations
	t.Run("PackageSwift_LinkAlternate", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		link := resp.Header.Get("Link")
		if link == "" || !strings.Contains(link, "alternate") {
			t.Fatalf("expected Link with alternate relation, got %q", link)
		}
		if !strings.Contains(link, "Package@swift-5.7.0.swift") {
			t.Fatalf("Link missing Package@swift-5.7.0.swift filename: %q", link)
		}
	})

	// Spec 4.3.1: swift-version query selects a specific manifest variant
	t.Run("PackageSwiftVariant", func(t *testing.T) {
		url := env.registryPath(spmE2EScope, spmTestPackage, spmVersion1, "Package.swift") + "?swift-version=5.7.0"
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest variant: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeSwift {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeSwift)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd == "" || !strings.Contains(cd, "Package@swift-5.7.0.swift") {
			t.Fatalf("Content-Disposition: got %q, want filename Package@swift-5.7.0.swift", cd)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:5.7.0")) {
			t.Fatalf("variant manifest missing swift-tools-version:5.7.0: %s", string(body))
		}
	})

	// Spec 4.3.1: server SHOULD respond 303 or 404 when variant does not exist.
	// Our mock upstream returns 404 for unknown swift-versions; the proxy propagates it.
	t.Run("PackageSwift_SwiftVersionNotPresent_Returns404", func(t *testing.T) {
		url := env.registryPath(spmE2EScope, spmTestPackage, spmVersion1, "Package.swift") + "?swift-version=99.0"
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404 for missing swift-version variant, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// Spec: server SHOULD respond to HEAD requests
	t.Run("HEAD_ReleaseMetadata", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodHead, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("HEAD version info: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HEAD expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("HEAD Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("HEAD Content-Version: got %q, want %q", v, contentVersion)
		}
	})

	t.Run("HEAD_PackageSwift", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodHead, env.registryPath(spmE2EScope, spmTestPackage, spmVersion1, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("HEAD manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HEAD Package.swift expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeSwift {
			t.Fatalf("HEAD Content-Type: got %q, want %q", ct, contentTypeSwift)
		}
	})

	// Spec 4.5: GET /identifiers?url=...
	t.Run("LookupIdentifiers", func(t *testing.T) {
		lookupURL := env.registryPath("identifiers") + "?url=https%3A%2F%2Fgithub.com%2Fexample%2F" + spmTestPackage
		req, _ := http.NewRequest(http.MethodGet, lookupURL, nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("lookup: expected 200, got %d: %s", resp.StatusCode, string(body))
		}
		body, _ := io.ReadAll(resp.Body)
		var out struct {
			Identifiers []string `json:"identifiers"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("parse lookup response: %v", err)
		}
		if len(out.Identifiers) == 0 {
			t.Fatal("expected at least one identifier")
		}
		if out.Identifiers[0] != spmE2EScope+"."+spmTestPackage {
			t.Fatalf("unexpected identifier: %v", out.Identifiers)
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
	})

	// Error cases (spec 3.3, 4.x)
	t.Run("List_NonExistentPackage_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, nonexistentPackage), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("Content-Type: got %q, want application/problem+json", ct)
		}
	})

	t.Run("Info_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, nonexistentVersion), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("version info: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("DownloadSourceArchive_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, nonexistentVersion)+".zip", nil)
		req.Header.Set("Accept", acceptZip)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("PackageSwift_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage, nonexistentVersion, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// Spec 3.4: wrong Accept version returns 4xx
	t.Run("WrongAccept_Returns4xx", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmTestPackage), nil)
		req.Header.Set("Accept", "application/vnd.swift.registry.v99+json")
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnsupportedMediaType {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 400 or 415, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("Content-Type: got %q, want application/problem+json", ct)
		}
	})

	// Spec 4.5: missing url query parameter returns 400
	t.Run("Lookup_NoURL_Returns400", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, env.registryPath("identifiers"), nil)
		req.Header.Set("Accept", acceptJSON)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(body))
		}
	})
}

// TestRegistryE2ESPMSplitMode tests split mode: signatures and package-collection index data
// are stored in a local file store, while source archives and manifests are proxied from the
// upstream.
//
// Two sub-test suites are run:
//   - StoreSignings: a pre-populated .sig file in the local store is served via download
//     headers and included in version-info signing metadata.
//   - StoreIndex: a pre-populated Package.json in the local store is returned by the
//     collection endpoints.
//
// Run with: go test -tags=e2e -v ./e2e/... -run TestRegistryE2ESPMSplitMode
func TestRegistryE2ESPMSplitMode(t *testing.T) {
	env := setupE2E(t)
	env.registryURL = "http://127.0.0.1:8082"
	env.useHTTPS = false
	env.httpClient = &http.Client{Timeout: 30 * time.Second}

	zipSigned := createMinimalZip(t, spmE2EScope, spmSignedPackage, spmSignedVersion, false)
	h := sha256.Sum256(zipSigned)
	checksumHex := fmt.Sprintf("%x", h[:])

	packageSwift := fmt.Sprintf(
		"// swift-tools-version:6.0\nimport PackageDescription\nlet package = Package(name: %q, products: [.library(name: %q, targets: [%q])], targets: [.target(name: %q)])\n",
		spmSignedPackage, spmSignedPackage, spmSignedPackage, spmSignedPackage,
	)

	upstream := httptest.NewServer(mockSPMUpstreamHandler(
		spmE2EScope, spmSignedPackage, spmSignedVersion,
		zipSigned, checksumHex, packageSwift, "" /* no variant */,
	))
	defer upstream.Close()

	// ── StoreSignings ────────────────────────────────────────────────────────────
	// Pre-populate a signature file in the local store and verify the proxy serves
	// it as X-Swift-Package-Signature on download and as signing in version info.
	t.Run("StoreSignings", func(t *testing.T) {
		localPath := t.TempDir()

		// Signature file path mirrors the FileRepo layout:
		// {localPath}/{scope}/{name}/{version}/{scope}.{name}-{version}.sig
		sigDir := filepath.Join(localPath, spmE2EScope, spmSignedPackage, spmSignedVersion)
		if err := os.MkdirAll(sigDir, 0755); err != nil {
			t.Fatalf("create sig dir: %v", err)
		}
		dummySig := make([]byte, 32)
		for i := range dummySig {
			dummySig[i] = byte(i + 50)
		}
		sigFileName := fmt.Sprintf("%s.%s-%s.sig", spmE2EScope, spmSignedPackage, spmSignedVersion)
		if err := os.WriteFile(filepath.Join(sigDir, sigFileName), dummySig, 0644); err != nil {
			t.Fatalf("write sig file: %v", err)
		}
		expectedSigB64 := base64.StdEncoding.EncodeToString(dummySig)

		env.configPath = writeSPMProxyConfig(t, upstream.URL, localPath, true /* storeSignings */, false)
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)

		t.Run("Download_WithSignature", func(t *testing.T) {
			url := env.registryPath(spmE2EScope, spmSignedPackage, spmSignedVersion) + ".zip"
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			req.Header.Set("Accept", acceptZip)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("download: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("download: expected 200, got %d: %s", resp.StatusCode, string(body))
			}
			sigFormat := resp.Header.Get("X-Swift-Package-Signature-Format")
			if sigFormat != "cms-1.0.0" {
				t.Fatalf("X-Swift-Package-Signature-Format: got %q, want cms-1.0.0", sigFormat)
			}
			sigHeader := resp.Header.Get("X-Swift-Package-Signature")
			if sigHeader == "" {
				t.Fatal("X-Swift-Package-Signature header missing")
			}
			if sigHeader != expectedSigB64 {
				t.Fatalf("X-Swift-Package-Signature: got %q, want %q", sigHeader, expectedSigB64)
			}
			// Archive content must still match what the upstream serves
			body, _ := io.ReadAll(resp.Body)
			if !bytes.Equal(body, zipSigned) {
				t.Fatal("download content does not match expected zip")
			}
		})

		t.Run("ReleaseMetadata_WithSigning", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, env.registryPath(spmE2EScope, spmSignedPackage, spmSignedVersion), nil)
			req.Header.Set("Accept", acceptJSON)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("version info: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
			}
			body, _ := io.ReadAll(resp.Body)
			var info map[string]any
			if err := json.Unmarshal(body, &info); err != nil {
				t.Fatalf("parse version info: %v", err)
			}
			resources, _ := info["resources"].([]any)
			if len(resources) == 0 {
				t.Fatal("no resources in version info")
			}
			r0, _ := resources[0].(map[string]any)
			signing, _ := r0["signing"].(map[string]any)
			if signing == nil {
				t.Fatalf("signing object missing in release metadata resources: %v", r0)
			}
			sigB64, _ := signing["signatureBase64Encoded"].(string)
			if sigB64 == "" {
				t.Fatal("signatureBase64Encoded empty")
			}
			if sigB64 != expectedSigB64 {
				t.Fatalf("signatureBase64Encoded: got %q, want %q", sigB64, expectedSigB64)
			}
			sigFormat, _ := signing["signatureFormat"].(string)
			if sigFormat != "cms-1.0.0" {
				t.Fatalf("signatureFormat: got %q, want cms-1.0.0", sigFormat)
			}
		})
	})

	// ── StoreIndex ───────────────────────────────────────────────────────────────
	// Pre-populate Package.json in the local store and verify the proxy's collection
	// endpoints return that package in both the scope-level and global collections.
	t.Run("StoreIndex", func(t *testing.T) {
		localPath := t.TempDir()

		// Package.json path mirrors the FileRepo layout:
		// {localPath}/{scope}/{name}/{version}/Package.json
		pkgDir := filepath.Join(localPath, spmE2EScope, spmSignedPackage, spmSignedVersion)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatalf("create pkg dir: %v", err)
		}
		packageJSON := fmt.Sprintf(
			`{"name":%q,"version":%q,"targets":[{"name":%q,"type":"regular"}],"products":[{"name":%q,"targets":[%q],"type":{"library":["automatic"]}}]}`,
			spmSignedPackage, spmSignedVersion, spmSignedPackage, spmSignedPackage, spmSignedPackage,
		)
		if err := os.WriteFile(filepath.Join(pkgDir, "Package.json"), []byte(packageJSON), 0644); err != nil {
			t.Fatalf("write Package.json: %v", err)
		}

		env.configPath = writeSPMProxyConfig(t, upstream.URL, localPath, false, true /* storeIndex */)
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)

		// Scope collection: GET /collection/{scope}
		t.Run("ScopeCollection_ContainsLocalPackage", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, env.registryPath("collection", spmE2EScope), nil)
			req.Header.Set("Accept", "application/json")
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("scope collection: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("scope collection: expected 200, got %d: %s", resp.StatusCode, string(body))
			}
			body, _ := io.ReadAll(resp.Body)
			if !bytes.Contains(body, []byte(spmE2EScope+"."+spmSignedPackage)) {
				t.Fatalf("scope collection missing %s.%s: %s", spmE2EScope, spmSignedPackage, string(body))
			}
		})

		// Global collection: GET /collection
		t.Run("GlobalCollection_ContainsLocalPackage", func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, env.registryPath("collection"), nil)
			req.Header.Set("Accept", "application/json")
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("global collection: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("global collection: expected 200, got %d: %s", resp.StatusCode, string(body))
			}
			body, _ := io.ReadAll(resp.Body)
			if !bytes.Contains(body, []byte(spmSignedPackage)) {
				t.Fatalf("global collection missing %s: %s", spmSignedPackage, string(body))
			}
		})
	})
}
