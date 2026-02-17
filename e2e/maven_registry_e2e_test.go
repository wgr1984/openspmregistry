//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the Maven-backed registry via HTTP API only.
// Run with: go test -tags=e2e -v ./e2e/... -run TestMavenRegistryE2E
// Prerequisites: Nexus running (make test-integration-up). Swift toolchain not required.
package e2e

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	mavenE2EScope     = "e2emaven"
	mavenTestPackage  = "TestPackage"
	mavenOtherPackage = "OtherPackage"
	mavenVersion1     = "1.0.0"
	mavenVersion1_1   = "1.1.0" // second version so GET Package.swift returns Link with alternate(s)
	mavenVersion2     = "2.0.0"

	// Swift Package Registry spec 3.5: server MUST set these response headers
	contentTypeJSON  = "application/json"
	contentTypeSwift = "text/x-swift"
	contentTypeZip   = "application/zip"
	contentVersion   = "1"
)

// createMinimalZip returns a zip containing scope.name/Package.swift, Package.json,
// and optionally Package@swift-5.7.0.swift and Sources/... for manifest extraction.
// Package.json is required for collection listing.
func createMinimalZip(t *testing.T, scope, name, version string, withVariant bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	dirPrefix := scope + "." + name + "/"

	swift, _ := w.Create(dirPrefix + "Package.swift")
	swift.Write([]byte("// swift-tools-version:5.3\nlet package = Package(name: \"test\")"))

	packageJSON := fmt.Sprintf(`{"name":"%s","version":"%s"}`, name, version)
	pj, _ := w.Create(dirPrefix + "Package.json")
	pj.Write([]byte(packageJSON))

	if withVariant {
		variant, _ := w.Create(dirPrefix + "Package@swift-5.7.0.swift")
		variant.Write([]byte("// swift-tools-version:5.7.0\nlet package = Package(name: \"test\")"))
	}

	src, _ := w.Create(dirPrefix + "Sources/" + name + "/" + name + ".swift")
	src.Write([]byte("public struct " + name + " {}"))

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// registryPublishMultipart uploads a package via the registry PUT publish endpoint.
// Parts: source-archive (required), optional metadata (metadata.json), optional metadata-signature.
func registryPublishMultipart(t *testing.T, env *e2eEnv, scope, pkg, version string, sourceZip []byte, metadataJSON []byte) {
	t.Helper()
	var body bytes.Buffer
	mp := multipart.NewWriter(&body)

	// source-archive
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="source-archive"; filename="source-archive.zip"`)
	h.Set("Content-Type", "application/zip")
	part, err := mp.CreatePart(h)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(sourceZip); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	if len(metadataJSON) > 0 {
		h2 := make(textproto.MIMEHeader)
		h2.Set("Content-Disposition", `form-data; name="metadata"; filename="metadata.json"`)
		h2.Set("Content-Type", "application/json")
		part2, err := mp.CreatePart(h2)
		if err != nil {
			t.Fatalf("create metadata part: %v", err)
		}
		if _, err := part2.Write(metadataJSON); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
	}

	if err := mp.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	url := env.registryPath(scope, pkg, version)
	req, err := http.NewRequest("PUT", url, &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mp.FormDataContentType())
	req.Header.Set("Accept", acceptJSON)
	env.setAuth(req)

	resp, err := env.httpClient.Do(req)
	if err != nil {
		t.Fatalf("publish request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("publish: status %d, body %s", resp.StatusCode, string(b))
	}
	// Spec 3.5, 4.6: Content-Version and Location
	if v := resp.Header.Get("Content-Version"); v != contentVersion {
		t.Fatalf("publish: Content-Version: got %q, want %q", v, contentVersion)
	}
	if resp.Header.Get("Location") == "" {
		t.Fatal("publish: Location header missing")
	}
}

// TestMavenRegistryE2E exercises the Maven-backed registry via HTTP API only.
//
// It covers all six Swift Package Registry normative endpoints (Registry spec 4):
//   - [1] GET /{scope}/{name} — List: subtest "List"
//   - [2] GET /{scope}/{name}/{version} — Fetch release metadata: "Exists", "Get", "Checksum", "PublishDate", "Metadata"
//   - [3] GET /{scope}/{name}/{version}/Package.swift{?swift-version} — Fetch manifest: "PackageSwift", "PackageSwiftVariant"
//   - [4] GET /{scope}/{name}/{version}.zip — Download source archive: "DownloadSourceArchive"
//   - [5] GET /identifiers{?url} — Lookup identifiers: "LookupIdentifiers"
//   - [6] PUT /{scope}/{name}/{version} — Create release: "Publish", "SecondPackage"
//
// Additional checks: "Collection" (package collections), "Cleanup" (Nexus).
func TestMavenRegistryE2E(t *testing.T) {
	env := setupE2E(t)
	waitForNexus(t, env)

	cleanNexusScope(t, env, mavenE2EScope, []string{mavenTestPackage, mavenOtherPackage})
	defer startRegistryServer(t, env)()

	zip1 := createMinimalZip(t, mavenE2EScope, mavenTestPackage, mavenVersion1, true)
	metadataBody := []byte(`{"description":"E2E test metadata"}`)

	t.Run("Publish", func(t *testing.T) {
		registryPublishMultipart(t, env, mavenE2EScope, mavenTestPackage, mavenVersion1, zip1, metadataBody)
	})

	time.Sleep(500 * time.Millisecond)

	// Publish second version of TestPackage so GET Package.swift returns Link header with alternate(s)
	t.Run("PublishSecondVersion", func(t *testing.T) {
		zip1_1 := createMinimalZip(t, mavenE2EScope, mavenTestPackage, mavenVersion1_1, false)
		registryPublishMultipart(t, env, mavenE2EScope, mavenTestPackage, mavenVersion1_1, zip1_1, metadataBody)
	})

	time.Sleep(500 * time.Millisecond)

	t.Run("Exists", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		// Spec 3.5: Content-Type, Content-Version (4.2 release metadata)
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
	})

	t.Run("Get", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		if info["id"] != mavenE2EScope+"."+mavenTestPackage || info["version"] != mavenVersion1 {
			t.Fatalf("unexpected id/version: %v", info)
		}
	})

	t.Run("Checksum", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		resources, _ := info["resources"].([]any)
		if len(resources) == 0 {
			t.Fatal("no resources")
		}
		r0, _ := resources[0].(map[string]any)
		checksum, _ := r0["checksum"].(string)
		if checksum == "" {
			t.Fatal("checksum empty")
		}
		hash := sha256.Sum256(zip1)
		expected := fmt.Sprintf("%x", hash[:])
		if checksum != expected {
			t.Fatalf("checksum: got %s, expected %s", checksum, expected)
		}
	})

	t.Run("PublishDate", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
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
		pt, err := time.Parse(time.RFC3339, publishedAt)
		if err != nil {
			t.Fatalf("parse publishedAt: %v", err)
		}
		if d := time.Since(pt); d < 0 || d > 2*time.Minute {
			t.Fatalf("publishedAt not recent: %v", pt)
		}
	})

	t.Run("PackageSwift", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		// Spec 4.3: Content-Type text/x-swift, Content-Version, Content-Disposition, Link (alternates)
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeSwift {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeSwift)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd == "" || !strings.Contains(cd, "attachment") || !strings.Contains(cd, "Package.swift") {
			t.Fatalf("Content-Disposition: got %q, want attachment; filename=\"Package.swift\"", cd)
		}
		// Spec 4.3: server MUST include Link with alternate relation when alternative manifests exist
		if link := resp.Header.Get("Link"); link == "" || !strings.Contains(link, "alternate") {
			t.Fatalf("Link header with alternate relation missing: %q", link)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:5.3")) {
			t.Fatalf("manifest missing swift-tools-version:5.3: %s", string(body))
		}
	})

	t.Run("PackageSwiftVariant", func(t *testing.T) {
		// Spec 4.3.1: swift-version query parameter
		manifestURL := env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1, "Package.swift") + "?swift-version=5.7.0"
		req, _ := http.NewRequest("GET", manifestURL, nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest variant: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeSwift {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeSwift)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd == "" || !strings.Contains(cd, "Package@swift-5.7.0.swift") {
			t.Fatalf("Content-Disposition: got %q, want filename Package@swift-5.7.0.swift", cd)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:5.7")) {
			t.Fatalf("variant missing 5.7: %s", string(body))
		}
	})

	t.Run("DownloadSourceArchive", func(t *testing.T) {
		// Spec 4.4: GET /{scope}/{name}/{version}.zip
		url := env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1) + ".zip"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Accept", acceptZip)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("download source archive: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("download: expected 200, got %d, body %s", resp.StatusCode, string(b))
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read download body: %v", err)
		}
		if len(body) != len(zip1) {
			t.Fatalf("download size: got %d, expected %d", len(body), len(zip1))
		}
		if !bytes.Equal(body, zip1) {
			t.Fatal("download content does not match published zip")
		}
		// Spec 3.5, 4.4: Content-Type, Content-Version, Content-Disposition, Cache-Control; 4.4.1 Digest
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
		if digest := resp.Header.Get("Digest"); digest != "" {
			hash := sha256.Sum256(zip1)
			expected := "sha-256=" + fmt.Sprintf("%x", hash[:])
			if digest != expected {
				t.Fatalf("Digest header: got %s, expected %s", digest, expected)
			}
		}
	})

	t.Run("LookupIdentifiers", func(t *testing.T) {
		// Spec 4.5: GET /identifiers?url=...
		lookupURL := env.registryPath("identifiers") + "?url=" + url.QueryEscape(env.registryURL)
		req, _ := http.NewRequest("GET", lookupURL, nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("lookup identifiers: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("lookup: expected 200, got %d, body %s", resp.StatusCode, string(b))
		}
		body, _ := io.ReadAll(resp.Body)
		var out struct {
			Identifiers []string `json:"identifiers"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("parse lookup response: %v", err)
		}
		if out.Identifiers == nil {
			t.Fatal("identifiers field missing or null")
		}
		// Spec 3.5: Content-Type, Content-Version
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
	})

	t.Run("Metadata", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var info map[string]any
		if err := json.Unmarshal(body, &info); err != nil {
			t.Fatalf("parse version info: %v", err)
		}
		meta, _ := info["metadata"].(map[string]any)
		if meta == nil {
			t.Fatal("metadata missing")
		}
		desc, _ := meta["description"].(string)
		if desc != "E2E test metadata" {
			t.Fatalf("metadata description: got %q", desc)
		}
	})

	t.Run("SecondPackage", func(t *testing.T) {
		zip2 := createMinimalZip(t, mavenE2EScope, mavenOtherPackage, mavenVersion2, false)
		registryPublishMultipart(t, env, mavenE2EScope, mavenOtherPackage, mavenVersion2, zip2, nil)
	})

	t.Run("List", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("list: expected 200, got %d", resp.StatusCode)
		}
		// Spec 3.5, 4.1: Content-Type, Content-Version, Link (latest-version)
		if ct := resp.Header.Get("Content-Type"); ct != contentTypeJSON {
			t.Fatalf("Content-Type: got %q, want %q", ct, contentTypeJSON)
		}
		if v := resp.Header.Get("Content-Version"); v != contentVersion {
			t.Fatalf("Content-Version: got %q, want %q", v, contentVersion)
		}
		if link := resp.Header.Get("Link"); link == "" || !strings.Contains(link, "latest-version") {
			t.Fatalf("Link header with latest-version missing: %q", link)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte(`"`+mavenVersion1+`"`)) {
			t.Fatalf("list missing version %s: %s", mavenVersion1, string(body))
		}

		req2, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenOtherPackage), nil)
		req2.Header.Set("Accept", acceptJSON)
		env.setAuth(req2)
		resp2, err := env.httpClient.Do(req2)
		if err != nil {
			t.Fatalf("list other: %v", err)
		}
		defer resp2.Body.Close()
		body2, _ := io.ReadAll(resp2.Body)
		if !bytes.Contains(body2, []byte(`"`+mavenVersion2+`"`)) {
			t.Fatalf("list other missing version %s: %s", mavenVersion2, string(body2))
		}
	})

	t.Run("Collection", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("collection"), nil)
		req.Header.Set("Accept", "application/json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("collection: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		for _, pkg := range []string{mavenE2EScope + "." + mavenTestPackage, mavenE2EScope + "." + mavenOtherPackage} {
			if !bytes.Contains(body, []byte(pkg)) {
				t.Fatalf("collection missing %s", pkg)
			}
		}

		reqScope, _ := http.NewRequest("GET", env.registryPath("collection", mavenE2EScope), nil)
		reqScope.Header.Set("Accept", "application/json")
		env.setAuth(reqScope)
		respScope, err := env.httpClient.Do(reqScope)
		if err != nil {
			t.Fatalf("scope collection: %v", err)
		}
		defer respScope.Body.Close()
		bodyScope, _ := io.ReadAll(respScope.Body)
		if !strings.Contains(string(bodyScope), mavenTestPackage) || !strings.Contains(string(bodyScope), mavenOtherPackage) {
			t.Fatalf("scope collection missing packages: %s", string(bodyScope))
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		cleanNexusScope(t, env, mavenE2EScope, []string{mavenTestPackage, mavenOtherPackage})
	})
}
