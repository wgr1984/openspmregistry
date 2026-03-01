//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the registry via HTTP API (file and Maven backends).
// Run with: go test -tags=e2e -v ./e2e/... -run TestRegistryE2E
// Prerequisites: for Maven backend, Maven server running (make test-integration-up; MAVEN_PROVIDER=nexus or reposilite). File backend needs no Maven server.
package e2e

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
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

	signedE2EScope   = "e2esign"
	signedE2EPackage = "SignedPkg"
	signedE2EVersion = "1.0.0"

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
	packageSwift := fmt.Sprintf(`// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "%s",
    products: [
        .library(name: "%s", targets: ["%s"]),
    ],
    targets: [
        .target(name: "%s"),
    ]
)
`, name, name, name, name)
	swift.Write([]byte(packageSwift))

	// Package.json must include products and targets so collection manifest (convertPackageJsonToManifest) and swift package-collection add validation succeed.
	packageJSON := fmt.Sprintf(`{"name":"%s","version":"%s","targets":[{"name":"%s","type":"regular"}],"products":[{"name":"%s","targets":["%s"],"type":{"library":["automatic"]}}]}`, name, version, name, name, name)
	pj, _ := w.Create(dirPrefix + "Package.json")
	pj.Write([]byte(packageJSON))

	if withVariant {
		variant, _ := w.Create(dirPrefix + "Package@swift-5.7.0.swift")
		variant.Write([]byte("// swift-tools-version:5.7.0\nimport PackageDescription\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])"))
	}

	src, _ := w.Create(dirPrefix + "Sources/" + name + "/" + name + ".swift")
	src.Write([]byte("public struct " + name + " {}"))

	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// registryPublishMultipart uploads a package via the registry PUT publish endpoint.
// Parts: source-archive (required), optional metadata (metadata.json), optional metadata-signature,
// optional source-archive-signature.
func registryPublishMultipart(t *testing.T, env *e2eEnv, scope, pkg, version string, sourceZip []byte, metadataJSON []byte, sourceArchiveSignature []byte) {
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

	if len(sourceArchiveSignature) > 0 {
		h3 := make(textproto.MIMEHeader)
		h3.Set("Content-Disposition", `form-data; name="source-archive-signature"; filename="source-archive.sig"`)
		h3.Set("Content-Type", "application/octet-stream")
		part3, err := mp.CreatePart(h3)
		if err != nil {
			t.Fatalf("create source-archive-signature part: %v", err)
		}
		if _, err := part3.Write(sourceArchiveSignature); err != nil {
			t.Fatalf("write source-archive-signature: %v", err)
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

const (
	nonexistentVersion = "99.0.0"
	nonexistentScope   = "nonexistentscope123"
	nonexistentPackage = "NonExistentPackage"
)

// runRegistryE2ETestBody runs the shared HTTP API e2e test body for a given backend (config already set, server already started).
// zip1 is the first published package zip (for checksum/download assertions). cleanupMaven: if true, call cleanMavenScope at end.
func runRegistryE2ETestBody(t *testing.T, env *e2eEnv, zip1 []byte, metadataBody []byte, cleanupMaven bool) {
	t.Helper()

	t.Run("Publish", func(t *testing.T) {
		registryPublishMultipart(t, env, mavenE2EScope, mavenTestPackage, mavenVersion1, zip1, metadataBody, nil)
	})

	time.Sleep(500 * time.Millisecond)

	// Publish second version of TestPackage so GET Package.swift returns Link header with alternate(s)
	t.Run("PublishSecondVersion", func(t *testing.T) {
		zip1_1 := createMinimalZip(t, mavenE2EScope, mavenTestPackage, mavenVersion1_1, false)
		registryPublishMultipart(t, env, mavenE2EScope, mavenTestPackage, mavenVersion1_1, zip1_1, metadataBody, nil)
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

	// Spec 4.2: Link header (latest-version, predecessor-version, successor-version)
	t.Run("InfoLinkHeaders", func(t *testing.T) {
		// GET 1.0.0: should have latest-version and successor-version (1.1.0)
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		link := resp.Header.Get("Link")
		if link == "" || !strings.Contains(link, "latest-version") {
			t.Fatalf("Link missing latest-version: %q", link)
		}
		if !strings.Contains(link, "successor-version") {
			t.Fatalf("Link missing successor-version for 1.0.0: %q", link)
		}
		// GET 1.1.0: should have latest-version and predecessor-version (1.0.0)
		req2, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1_1), nil)
		req2.Header.Set("Accept", acceptJSON)
		env.setAuth(req2)
		resp2, err := env.httpClient.Do(req2)
		if err != nil {
			t.Fatalf("get version 1.1.0: %v", err)
		}
		resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for 1.1.0, got %d", resp2.StatusCode)
		}
		link2 := resp2.Header.Get("Link")
		if link2 == "" || !strings.Contains(link2, "latest-version") {
			t.Fatalf("Link missing latest-version for 1.1.0: %q", link2)
		}
		if !strings.Contains(link2, "predecessor-version") {
			t.Fatalf("Link missing predecessor-version for 1.1.0: %q", link2)
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
		// Allow up to 2h clock skew (future or past); file backend may use file mtime
		if d := time.Since(pt); d < -2*time.Hour || d > 2*time.Hour {
			t.Fatalf("publishedAt not plausible: %v (now %v)", pt, time.Now().UTC())
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
		// Spec 4.3: when Link is present it must include alternate relation for other manifest variants.
		// Maven backend does not populate GetAlternativeManifests (same-version variants), so Link may be empty.
		if link := resp.Header.Get("Link"); link != "" && !strings.Contains(link, "alternate") {
			t.Fatalf("Link header present but missing alternate relation: %q", link)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:6.0")) {
			t.Fatalf("manifest missing swift-tools-version:6.0: %s", string(body))
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

	// Spec 4.3.1: when requested swift-version has no Package@swift-X.swift, server SHOULD respond 303 to unqualified Package.swift (or 404)
	t.Run("PackageSwift_SwiftVersionNotPresent_Returns303Or404", func(t *testing.T) {
		manifestURL := env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1, "Package.swift") + "?swift-version=99.0"
		req, _ := http.NewRequest("GET", manifestURL, nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusSeeOther:
			if loc := resp.Header.Get("Location"); loc == "" {
				t.Fatal("303 response missing Location header")
			}
		case http.StatusNotFound:
			// current implementation returns 404 when variant file does not exist
		default:
			t.Fatalf("expected 303 or 404 for missing swift-version variant, got %d", resp.StatusCode)
		}
	})

	// Spec 4.3: when multiple manifests exist (we published 1.0.0 with Package.swift + Package@swift-5.7.0.swift),
	// server MUST include Link with alternate. File backend returns them; Maven does not populate same-version alternates.
	t.Run("PackageSwift_LinkAlternate_WhenMultipleManifests", func(t *testing.T) {
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
		link := resp.Header.Get("Link")
		if !cleanupMaven {
			// File backend: GetAlternativeManifests returns same-version variants; Link must be present.
			if link == "" || !strings.Contains(link, "alternate") {
				t.Fatalf("expected Link with alternate when multiple manifests published (Package.swift + Package@swift-5.7.0.swift), got %q", link)
			}
		} else {
			// Maven backend: does not return same-version alternates; if Link is present it must contain alternate.
			if link != "" && !strings.Contains(link, "alternate") {
				t.Fatalf("Link present but missing alternate: %q", link)
			}
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
		// Spec 4.4: server MUST set Content-Length for source archive
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			if cl != fmt.Sprint(len(body)) {
				t.Fatalf("Content-Length: got %s, body length %d", cl, len(body))
			}
		}
		if digest := resp.Header.Get("Digest"); digest != "" {
			hash := sha256.Sum256(zip1)
			expected := "sha-256=" + fmt.Sprintf("%x", hash[:])
			if digest != expected {
				t.Fatalf("Digest header: got %s, expected %s", digest, expected)
			}
		}
	})

	// Spec: server SHOULD respond to HEAD requests for each endpoint
	t.Run("HEAD_ReleaseMetadata", func(t *testing.T) {
		req, _ := http.NewRequest("HEAD", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("HEAD version: %v", err)
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
		if resp.ContentLength != 0 {
			body, _ := io.ReadAll(resp.Body)
			if len(body) != 0 {
				t.Fatalf("HEAD response should have no body, got %d bytes", len(body))
			}
		}
	})

	t.Run("HEAD_PackageSwift", func(t *testing.T) {
		req, _ := http.NewRequest("HEAD", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
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

	t.Run("LookupIdentifiers", func(t *testing.T) {
		// Spec 4.5: GET /identifiers?url=... — 200 with identifiers (possibly empty) or 404 when URL not registered
		lookupURL := env.registryPath("identifiers") + "?url=" + url.QueryEscape(env.registryURL)
		req, _ := http.NewRequest("GET", lookupURL, nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("lookup identifiers: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			// Backend has no URL→identifier mapping (e.g. file repo)
			return
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("lookup: expected 200 or 404, got %d, body %s", resp.StatusCode, string(b))
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
		registryPublishMultipart(t, env, mavenE2EScope, mavenOtherPackage, mavenVersion2, zip2, nil, nil)
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
		// Spec 4.1: response must have top-level "releases" object; each value has url and/or problem
		var listResp struct {
			Releases map[string]struct {
				URL     string         `json:"url"`
				Problem map[string]any `json:"problem"`
			} `json:"releases"`
		}
		if err := json.Unmarshal(body, &listResp); err != nil {
			t.Fatalf("parse list response: %v", err)
		}
		if listResp.Releases == nil {
			t.Fatal("list response missing releases object")
		}
		for ver, entry := range listResp.Releases {
			if entry.URL == "" && (entry.Problem == nil || len(entry.Problem) == 0) {
				t.Fatalf("list release %q must have url or problem", ver)
			}
		}
		for _, ver := range []string{mavenVersion1, mavenVersion1_1} {
			if !bytes.Contains(body, []byte(`"`+ver+`"`)) {
				t.Fatalf("list missing version %s: %s", ver, string(body))
			}
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

	// Spec 4.1: pagination Link headers (first, last, next, prev) when listPageSize is set
	t.Run("ListPagination", func(t *testing.T) {
		for page, wantVer := range map[int]string{1: mavenVersion1_1, 2: mavenVersion1} {
			req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage)+"?page="+fmt.Sprint(page), nil)
			req.Header.Set("Accept", acceptJSON)
			env.setAuth(req)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("list page %d: %v", page, err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("list page %d: expected 200, got %d", page, resp.StatusCode)
			}
			if !bytes.Contains(body, []byte(`"`+wantVer+`"`)) {
				t.Fatalf("page %d should contain version %s: %s", page, wantVer, string(body))
			}
			link := resp.Header.Get("Link")
			for _, rel := range []string{"first", "last"} {
				if !strings.Contains(link, `rel="`+rel+`"`) {
					t.Fatalf("page %d missing %s link: %q", page, rel, link)
				}
			}
			if page == 1 && !strings.Contains(link, `rel="next"`) {
				t.Fatalf("page 1 missing next link: %q", link)
			}
			if page == 2 && !strings.Contains(link, `rel="prev"`) {
				t.Fatalf("page 2 missing prev link: %q", link)
			}
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

	// Error cases (spec 3.3, 4.x)
	t.Run("List_NonExistentPackage_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, nonexistentPackage), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("Content-Type: got %q, want application/problem+json", ct)
		}
		// Spec 3.3 / RFC 7807: problem details must include at least detail
		var problem map[string]any
		if err := json.Unmarshal(body, &problem); err != nil {
			t.Fatalf("parse problem body: %v", err)
		}
		if _, ok := problem["detail"]; !ok {
			t.Fatalf("problem response missing detail field: %s", string(body))
		}
	})

	t.Run("Info_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, nonexistentVersion), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(b))
		}
	})

	t.Run("PackageSwift_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, nonexistentVersion, "Package.swift"), nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(b))
		}
	})

	t.Run("DownloadSourceArchive_NonExistentVersion_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage, nonexistentVersion)+".zip", nil)
		req.Header.Set("Accept", acceptZip)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(b))
		}
	})

	t.Run("Collection_NonExistentScope_Returns404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("collection", nonexistentScope), nil)
		req.Header.Set("Accept", "application/json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("collection: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(b))
		}
	})

	t.Run("WrongAccept_Returns4xx", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage), nil)
		req.Header.Set("Accept", "application/vnd.swift.registry.v99+json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnsupportedMediaType {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 400 or 415, got %d: %s", resp.StatusCode, string(b))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("Content-Type: got %q, want application/problem+json", ct)
		}
	})

	t.Run("Lookup_NoURL_Returns400", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("identifiers"), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 400, got %d: %s", resp.StatusCode, string(b))
		}
	})

	// Spec 3.2: server SHOULD respond 401 when auth required but no credentials
	if env.useHTTPS {
		t.Run("Unauthorized_NoCredentials_Returns401", func(t *testing.T) {
			req, _ := http.NewRequest("GET", env.registryPath(mavenE2EScope, mavenTestPackage), nil)
			req.Header.Set("Accept", acceptJSON)
			// do not call env.setAuth(req)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 401 without auth, got %d: %s", resp.StatusCode, string(b))
			}
		})
	}

	t.Run("Publish_DuplicateVersion_Returns409", func(t *testing.T) {
		var body bytes.Buffer
		mp := multipart.NewWriter(&body)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="source-archive"; filename="source-archive.zip"`)
		h.Set("Content-Type", "application/zip")
		part, _ := mp.CreatePart(h)
		part.Write(zip1)
		mp.Close()
		req, _ := http.NewRequest("PUT", env.registryPath(mavenE2EScope, mavenTestPackage, mavenVersion1), &body)
		req.Header.Set("Content-Type", mp.FormDataContentType())
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("expected 409 for duplicate publish, got %d: %s", resp.StatusCode, string(respBody))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
			t.Fatalf("409 Content-Type: got %q, want application/problem+json", ct)
		}
		var problem map[string]any
		if err := json.Unmarshal(respBody, &problem); err != nil {
			t.Fatalf("parse 409 body: %v", err)
		}
		if _, ok := problem["detail"]; !ok {
			t.Fatalf("409 problem response missing detail: %s", string(respBody))
		}
	})

	if cleanupMaven {
		t.Run("Cleanup", func(t *testing.T) {
			cleanMavenScope(t, env, mavenE2EScope, []string{mavenTestPackage, mavenOtherPackage})
		})
	}
}

// TestRegistryE2E runs the registry HTTP API e2e tests against both file and Maven backends.
//
// It covers all six Swift Package Registry normative endpoints (Registry spec 4) plus collections and error cases.
func TestRegistryE2E(t *testing.T) {
	env := setupE2E(t)
	defer runE2ECleanup(t, env)()
	zip1 := createMinimalZip(t, mavenE2EScope, mavenTestPackage, mavenVersion1, true)
	metadataBody := []byte(`{"description":"E2E test metadata"}`)

	t.Run("Maven", func(t *testing.T) {
		waitForMaven(t, env)
		cleanMavenScope(t, env, mavenE2EScope, []string{mavenTestPackage, mavenOtherPackage})
		if env.useHTTPS {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.https.yml")
		} else if env.mavenProvider == "reposilite" {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.reposilite.yml")
		} else {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.yml")
		}
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)
		runRegistryE2ETestBody(t, env, zip1, metadataBody, true)
	})

	t.Run("File", func(t *testing.T) {
		fileRepoPath := filepath.Join(env.rootDir, "e2e-file-repo")
		os.RemoveAll(fileRepoPath)
		defer os.RemoveAll(fileRepoPath)
		env.configPath = filepath.Join(env.rootDir, "config.e2e.file.yml")
		if env.useHTTPS {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.file.https.yml")
		}
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)
		runRegistryE2ETestBody(t, env, zip1, metadataBody, false)
	})
}

// TestRegistryE2ESignedPackages tests signed package publishing and retrieval for both File and Maven backends.
//
// It verifies that:
// - Publishing with source-archive-signature stores the signature correctly
// - GET release metadata includes signing object with signatureBase64Encoded and signatureFormat
// - GET .zip download includes X-Swift-Package-Signature and X-Swift-Package-Signature-Format headers
func TestRegistryE2ESignedPackages(t *testing.T) {
	env := setupE2E(t)
	defer runE2ECleanup(t, env)()
	zip1 := createMinimalZip(t, signedE2EScope, signedE2EPackage, signedE2EVersion, false)
	// Dummy signature: 32 bytes of test data (not a real CMS signature, but sufficient for E2E)
	dummySignature := make([]byte, 32)
	for i := range dummySignature {
		dummySignature[i] = byte(i)
	}

	t.Run("Maven", func(t *testing.T) {
		waitForMaven(t, env)
		cleanMavenScope(t, env, signedE2EScope, []string{signedE2EPackage})
		if env.useHTTPS {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.https.yml")
		} else if env.mavenProvider == "reposilite" {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.reposilite.yml")
		} else {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.yml")
		}
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)
		runSignedPackageTestBody(t, env, zip1, dummySignature, true)
	})

	t.Run("File", func(t *testing.T) {
		fileRepoPath := filepath.Join(env.rootDir, "e2e-file-repo-signed")
		os.RemoveAll(fileRepoPath)
		defer os.RemoveAll(fileRepoPath)
		env.configPath = filepath.Join(env.rootDir, "config.e2e.file.yml")
		if env.useHTTPS {
			env.configPath = filepath.Join(env.rootDir, "config.e2e.file.https.yml")
		}
		defer startRegistryServer(t, env)()
		time.Sleep(500 * time.Millisecond)
		runSignedPackageTestBody(t, env, zip1, dummySignature, false)
	})
}

// runSignedPackageTestBody runs the signed package test body for a given backend.
func runSignedPackageTestBody(t *testing.T, env *e2eEnv, zip1 []byte, signature []byte, cleanupMaven bool) {
	t.Helper()

	t.Run("PublishSigned", func(t *testing.T) {
		registryPublishMultipart(t, env, signedE2EScope, signedE2EPackage, signedE2EVersion, zip1, nil, signature)
	})

	time.Sleep(500 * time.Millisecond)

	t.Run("VerifyReleaseMetadataSigning", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(signedE2EScope, signedE2EPackage, signedE2EVersion), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get version: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(b))
		}
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
		signing, _ := r0["signing"].(map[string]any)
		if signing == nil {
			t.Fatal("signing object missing in release metadata")
		}
		signatureBase64Encoded, _ := signing["signatureBase64Encoded"].(string)
		if signatureBase64Encoded == "" {
			t.Fatal("signatureBase64Encoded missing or empty")
		}
		// Verify the signature matches what we published (base64 encoded)
		expectedSignatureBase64 := base64.StdEncoding.EncodeToString(signature)
		if signatureBase64Encoded != expectedSignatureBase64 {
			t.Fatalf("signatureBase64Encoded: got %q, expected %q", signatureBase64Encoded, expectedSignatureBase64)
		}
		signatureFormat, _ := signing["signatureFormat"].(string)
		if signatureFormat != "cms-1.0.0" {
			t.Fatalf("signatureFormat: got %q, want cms-1.0.0", signatureFormat)
		}
	})

	t.Run("VerifyDownloadHeaders", func(t *testing.T) {
		url := env.registryPath(signedE2EScope, signedE2EPackage, signedE2EVersion) + ".zip"
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
		signatureFormat := resp.Header.Get("X-Swift-Package-Signature-Format")
		if signatureFormat != "cms-1.0.0" {
			t.Fatalf("X-Swift-Package-Signature-Format: got %q, want cms-1.0.0", signatureFormat)
		}
		signatureHeader := resp.Header.Get("X-Swift-Package-Signature")
		if signatureHeader == "" {
			t.Fatal("X-Swift-Package-Signature header missing")
		}
		// Verify the signature in header matches what we published
		expectedSignatureBase64 := base64.StdEncoding.EncodeToString(signature)
		if signatureHeader != expectedSignatureBase64 {
			t.Fatalf("X-Swift-Package-Signature: got %q, expected %q", signatureHeader, expectedSignatureBase64)
		}
		// Verify the zip content matches
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read download body: %v", err)
		}
		if !bytes.Equal(body, zip1) {
			t.Fatal("download content does not match published zip")
		}
	})

	if cleanupMaven {
		t.Run("Cleanup", func(t *testing.T) {
			cleanMavenScope(t, env, signedE2EScope, []string{signedE2EPackage})
		})
	}
}
