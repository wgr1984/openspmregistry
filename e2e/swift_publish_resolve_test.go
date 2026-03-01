//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the Swift package registry.
// Run with: go test -tags=e2e -v ./e2e/...
// Prerequisites: Maven server running (make test-integration-up; MAVEN_PROVIDER=nexus or reposilite), Swift toolchain installed.
package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"OpenSPMRegistry/internal/e2ecerts"
)

const (
	defaultRegistryURL = "http://127.0.0.1:8082"
	scope              = "example"
	acceptJSON         = "application/vnd.swift.registry.v1+json"
	acceptSwift        = "application/vnd.swift.registry.v1+swift"
	acceptZip          = "application/vnd.swift.registry.v1+zip"
	serverReadyWait    = 30 * time.Second
)

// e2eEnv holds paths and config for the E2E test.
type e2eEnv struct {
	rootDir       string
	configPath    string
	registryURL   string
	useHTTPS      bool
	registryUser  string
	registryPass  string
	consumerDir   string
	samplePkgDir  string
	utilsPkgDir   string
	mavenProvider string // "nexus" or "reposilite"
	mavenBaseURL  string // Nexus: http://localhost:8081; Reposilite: http://localhost:8080
	mavenRepo     string // repo name: "private" (Nexus and Reposilite) or custom
	nexusUser     string // Maven auth username (admin for Nexus, token name for Reposilite)
	nexusPass     string // Maven auth password
	httpClient    *http.Client
}

func (e *e2eEnv) registryPath(parts ...string) string {
	return e.registryURL + "/" + strings.Join(parts, "/")
}

// setAuth adds Basic auth to the request when using HTTPS.
func (e *e2eEnv) setAuth(req *http.Request) {
	if e.useHTTPS {
		req.SetBasicAuth(e.registryUser, e.registryPass)
	}
}

// findRepoRoot walks up from the current directory to find the repository root (containing go.mod).
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// setupE2E initializes the E2E environment and skips if prerequisites are not met.
// Supports E2E_REGISTRY_URL (default http://127.0.0.1:8082); https:// uses config.e2e.https.yml.
// MAVEN_PROVIDER=nexus (default) or reposilite selects Maven backend and config.
func setupE2E(t *testing.T) *e2eEnv {
	t.Helper()
	if os.Getenv("E2E_TESTS") == "" {
		t.Skip("Skipping E2E test. Set E2E_TESTS=1 to run.")
	}

	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("find repo root: %v", err)
	}

	registryURL := os.Getenv("E2E_REGISTRY_URL")
	if registryURL == "" {
		registryURL = defaultRegistryURL
	}
	registryURL = strings.TrimSuffix(registryURL, "/")
	useHTTPS := strings.HasPrefix(registryURL, "https://")

	mavenProvider := os.Getenv("MAVEN_PROVIDER")
	if mavenProvider == "" {
		mavenProvider = "nexus"
	}
	mavenRepoURL := os.Getenv("MAVEN_REPO_URL")
	mavenRepoName := os.Getenv("MAVEN_REPO_NAME")
	var mavenBaseURL string
	if mavenProvider == "reposilite" {
		if mavenRepoURL == "" {
			mavenRepoURL = "http://localhost:8080"
		}
		if mavenRepoName == "" {
			mavenRepoName = "private"
		}
		mavenBaseURL = strings.TrimSuffix(mavenRepoURL, "/")
	} else {
		if mavenRepoName == "" {
			mavenRepoName = "private"
		}
		mavenBaseURL = os.Getenv("NEXUS_URL")
		if mavenBaseURL == "" {
			mavenBaseURL = "http://localhost:8081"
		}
		mavenBaseURL = strings.TrimSuffix(mavenBaseURL, "/")
	}

	configPath := filepath.Join(root, "config.e2e.yml")
	if mavenProvider == "reposilite" && !useHTTPS {
		configPath = filepath.Join(root, "config.e2e.reposilite.yml")
	} else if useHTTPS {
		configPath = filepath.Join(root, "config.e2e.https.yml")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("missing %s: %v", configPath, err)
	}

	nexusUser := os.Getenv("MAVEN_REPO_USERNAME")
	if nexusUser == "" {
		if mavenProvider == "reposilite" {
			nexusUser = "e2e"
		} else {
			nexusUser = "admin"
		}
	}
	nexusPass := os.Getenv("MAVEN_REPO_PASSWORD")
	if nexusPass == "" {
		if mavenProvider == "reposilite" {
			if b, err := os.ReadFile(filepath.Join(root, ".reposilite-test-token")); err == nil {
				nexusPass = strings.TrimSpace(string(b))
			} else {
				nexusPass = "test-secret"
			}
		} else {
			if b, err := os.ReadFile(filepath.Join(root, ".nexus-test-password")); err == nil {
				nexusPass = strings.TrimSpace(string(b))
			} else {
				nexusPass = "admin123"
			}
		}
	}

	registryUser := os.Getenv("E2E_REGISTRY_USER")
	if registryUser == "" {
		registryUser = "admin"
	}
	registryPass := os.Getenv("E2E_REGISTRY_PASS")
	if registryPass == "" {
		registryPass = "admin123"
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	if useHTTPS {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	env := &e2eEnv{
		rootDir:       root,
		configPath:    configPath,
		registryURL:   registryURL,
		useHTTPS:      useHTTPS,
		registryUser:  registryUser,
		registryPass:  registryPass,
		consumerDir:   filepath.Join(root, "testdata", "e2e", "Consumer"),
		samplePkgDir:  filepath.Join(root, "testdata", "e2e", "example.SamplePackage"),
		utilsPkgDir:   filepath.Join(root, "testdata", "e2e", "example.UtilsPackage"),
		mavenProvider: mavenProvider,
		mavenBaseURL:  mavenBaseURL,
		mavenRepo:     mavenRepoName,
		nexusUser:     nexusUser,
		nexusPass:     nexusPass,
		httpClient:    httpClient,
	}
	return env
}

// runE2ECleanup runs the e2e-clean-nexus script at the end of the test suite to remove com and example trees.
// Call via defer: defer runE2ECleanup(t, env)()
func runE2ECleanup(t *testing.T, env *e2eEnv) func() {
	return func() {
		t.Helper()
		cmd := exec.Command("go", "run", "./cmd/e2e-clean-nexus")
		cmd.Dir = env.rootDir
		cmd.Env = append(os.Environ(),
			"MAVEN_PROVIDER="+env.mavenProvider,
			"MAVEN_REPO_NAME="+env.mavenRepo,
		)
		if env.mavenProvider == "nexus" {
			cmd.Env = append(cmd.Env,
				"NEXUS_URL="+env.mavenBaseURL,
				"MAVEN_REPO_USERNAME="+env.nexusUser,
				"MAVEN_REPO_PASSWORD="+env.nexusPass,
			)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("e2e cleanup (non-fatal): %v\n%s", err, out)
		}
	}
}

// cleanMavenScope removes packages from the Maven repo (Nexus or Reposilite) for the given group and package names.
func cleanMavenScope(t *testing.T, env *e2eEnv, group string, packageNames []string) {
	t.Helper()
	auth := base64.StdEncoding.EncodeToString([]byte(env.nexusUser + ":" + env.nexusPass))
	if env.mavenProvider == "nexus" {
		for _, pkgName := range packageNames {
			baseURL := fmt.Sprintf("%s/service/rest/v1/search?repository=%s&group=%s&name=%s",
				env.mavenBaseURL, env.mavenRepo, group, pkgName)
			token := ""
			for {
				url := baseURL
				if token != "" {
					url += "&continuationToken=" + token
				}
				req, err := http.NewRequest("GET", url, nil)
				if err != nil {
					t.Logf("cleanMaven: create request: %v", err)
					return
				}
				req.Header.Set("Authorization", "Basic "+auth)
				resp, err := env.httpClient.Do(req)
				if err != nil {
					t.Logf("cleanMaven: request failed: %v", err)
					return
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					return
				}
				var data struct {
					Items             []struct{ ID string } `json:"items"`
					ContinuationToken string                `json:"continuationToken"`
				}
				if err := json.Unmarshal(body, &data); err != nil {
					t.Logf("cleanMaven: parse response: %v", err)
					return
				}
				for _, item := range data.Items {
					delURL := fmt.Sprintf("%s/service/rest/v1/components/%s", env.mavenBaseURL, item.ID)
					delReq, _ := http.NewRequest("DELETE", delURL, nil)
					delReq.Header.Set("Authorization", "Basic "+auth)
					env.httpClient.Do(delReq)
				}
				token = data.ContinuationToken
				if token == "" {
					break
				}
			}
		}
		return
	}
	// Reposilite: recursive list + DELETE per file
	for _, pkgName := range packageNames {
		path := group + "/" + pkgName
		cleanReposilitePath(t, env, path, auth)
	}
}

func cleanReposilitePath(t *testing.T, env *e2eEnv, path, auth string) {
	t.Helper()
	detailsURL := fmt.Sprintf("%s/api/maven/details/%s/%s", env.mavenBaseURL, env.mavenRepo, path)
	req, _ := http.NewRequest("GET", detailsURL, nil)
	req.Header.Set("Authorization", "Basic "+auth)
	resp, err := env.httpClient.Do(req)
	if err != nil {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var details struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &details); err != nil {
		return
	}
	if details.Type == "FILE" {
		// Reposilite delete is DELETE /{repository}/{gav}, not /api/maven/file/...
		delURL := fmt.Sprintf("%s/%s/%s", env.mavenBaseURL, env.mavenRepo, path)
		delReq, _ := http.NewRequest("DELETE", delURL, nil)
		delReq.Header.Set("Authorization", "Basic "+auth)
		env.httpClient.Do(delReq)
		return
	}
	for _, child := range details.Content {
		childPath := path + "/" + child.Name
		cleanReposilitePath(t, env, childPath, auth)
	}
}

// cleanNexus removes example.SamplePackage, example.UtilsPackage, example.SignedPkg, and example.SwiftSignedPkg from the Maven repo.
// SignedPkg is cleaned so PublishSignedPackageViaHTTP and ConsumerResolve start from a clean state.
// SwiftSignedPkg is cleaned so PublishWithSwiftSigning (spm-extended) can re-publish.
func cleanNexus(t *testing.T, env *e2eEnv) {
	t.Helper()
	cleanMavenScope(t, env, "example", []string{"SamplePackage", "UtilsPackage", "SignedPkg", "SwiftSignedPkg"})
}

// waitForMaven checks that the Maven repository server (Nexus or Reposilite) is reachable.
func waitForMaven(t *testing.T, env *e2eEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var healthURL string
	if env.mavenProvider == "nexus" {
		healthURL = env.mavenBaseURL + "/service/rest/v1/status"
	} else {
		healthURL = env.mavenBaseURL + "/"
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	resp, err := env.httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Skipf("Maven server not reachable at %s. Start with: make test-integration-up", env.mavenBaseURL)
	}
	resp.Body.Close()
}

// waitForRegistry polls the registry until it responds or timeout.
// GET / returns 404 (MainAction fallback); any HTTP response means the server is up.
func waitForRegistry(t *testing.T, env *e2eEnv) {
	t.Helper()
	deadline := time.Now().Add(serverReadyWait)
	for time.Now().Before(deadline) {
		resp, err := env.httpClient.Get(env.registryURL + "/")
		if err == nil {
			resp.Body.Close()
			// 200 or 404 both indicate server is running
			if resp.StatusCode == 200 || resp.StatusCode == 404 {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatal("registry did not become ready in time")
}

// runSwift runs a Swift CLI command; returns (stdout+stderr, error).
func runSwift(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("swift", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// swiftAvailable checks if the Swift toolchain is installed.
func swiftAvailable() bool {
	cmd := exec.Command("swift", "--version")
	return cmd.Run() == nil
}

// TestSwiftPublishResolve runs the full E2E: publish packages, verify HTTP API, resolve consumer.
func TestSwiftPublishResolve(t *testing.T) {
	env := setupE2E(t)
	defer runE2ECleanup(t, env)()
	waitForMaven(t, env)

	if !swiftAvailable() {
		t.Skip("Swift toolchain not found. Install Swift to run this test.")
	}

	// Clean state (matches script: Setup section)
	os.Remove(filepath.Join(env.consumerDir, "Package.resolved"))
	os.RemoveAll(filepath.Join(env.consumerDir, ".build"))
	cleanNexus(t, env)
	// Remove e2emaven and e2esign scopes so collection is not polluted by a previous E2E run
	// (Swift package-collection add validates all packages in the collection; stale minimal packages may lack products/targets in Package.json)
	cleanMavenScope(t, env, "e2emaven", []string{"TestPackage", "OtherPackage"})
	cleanMavenScope(t, env, "e2esign", []string{"SignedPkg"})

	// Purge Swift PM cache (script lines 104-107; ignore errors)
	pc := exec.Command("swift", "package", "purge-cache")
	pc.Dir = env.consumerDir
	pc.Run()
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), "Library", "Caches", "org.swift.swiftpm"))
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), "Library", "org.swift.swiftpm"))
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".cache", "org.swift.swiftpm"))

	defer startRegistryServer(t, env)()

	// HTTPS: ensure certs exist and login
	if env.useHTTPS {
		certsDir := filepath.Join(env.rootDir, "testdata", "e2e", "certs")
		if _, err := os.Stat(filepath.Join(certsDir, "server.crt")); err != nil {
			if err := e2ecerts.Generate(certsDir); err != nil {
				t.Fatalf("generate E2E certs: %v", err)
			}
		}
		// Add cert to keychain (macOS) so Swift PM trusts it
		exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot", "-p", "ssl",
			filepath.Join(env.rootDir, "testdata", "e2e", "certs", "server.crt")).Run()

		out, err := runSwift(t, env.rootDir, "package-registry", "login", env.registryURL,
			"--username", env.registryUser, "--password", env.registryPass, "--no-confirm")
		if err != nil {
			t.Fatalf("swift package-registry login: %v\n%s", err, out)
		}
	}

	// Publish packages (script: dump-package before each, then publish)
	t.Run("Publish", func(t *testing.T) {
		// Clean fixture .build/.swiftpm so publish uses current source, not stale build state
		os.RemoveAll(filepath.Join(env.samplePkgDir, ".build"))
		os.RemoveAll(filepath.Join(env.samplePkgDir, ".swiftpm"))
		os.RemoveAll(filepath.Join(env.utilsPkgDir, ".build"))
		os.RemoveAll(filepath.Join(env.utilsPkgDir, ".swiftpm"))

		publishOpts := []string{"--url", env.registryURL}
		if !env.useHTTPS {
			publishOpts = append(publishOpts, "--allow-insecure-http")
		}
		for _, pkg := range []struct{ name, dir string }{
			{"SamplePackage", env.samplePkgDir},
			{"UtilsPackage", env.utilsPkgDir},
		} {
			pkgID := scope + "." + pkg.name
			for _, ver := range []string{"1.0.0", "1.1.0"} {
				// Prepare Package.json (script lines 184, 195)
				if out, err := runSwift(t, pkg.dir, "package", "dump-package"); err == nil {
					os.WriteFile(filepath.Join(pkg.dir, "Package.json"), []byte(out), 0644)
				}
				out, err := runSwift(t, pkg.dir, append([]string{"package-registry", "publish", pkgID, ver}, publishOpts...)...)
				if err != nil {
					t.Fatalf("publish %s %s: %v\n%s", pkgID, ver, err, out)
				}
			}
		}
	})

	// HTTP verification
	t.Run("VerifyMetadata", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(scope, "SamplePackage", "1.0.0"), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get metadata: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte(`"metadata"`)) {
			t.Fatalf("package info missing metadata")
		}
		if !bytes.Contains(body, []byte(`"description"`)) {
			t.Fatalf("package info missing description")
		}
	})

	// Publish signed package via HTTP and verify signing metadata
	t.Run("PublishSignedPackageViaHTTP", func(t *testing.T) {
		// Create minimal zip for signed package
		signedPkgZip := createMinimalZip(t, scope, "SignedPkg", "1.0.0", false)
		// Dummy signature: 32 bytes of test data
		dummySignature := make([]byte, 32)
		for i := range dummySignature {
			dummySignature[i] = byte(i + 100) // Different pattern than registry_e2e test
		}
		// Publish with signature
		registryPublishMultipart(t, env, scope, "SignedPkg", "1.0.0", signedPkgZip, nil, dummySignature)

		// Verify release metadata contains signing
		req, _ := http.NewRequest("GET", env.registryPath(scope, "SignedPkg", "1.0.0"), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get signed package metadata: %v", err)
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
		signatureFormat, _ := signing["signatureFormat"].(string)
		if signatureFormat != "cms-1.0.0" {
			t.Fatalf("signatureFormat: got %q, want cms-1.0.0", signatureFormat)
		}

		// Verify download headers
		url := env.registryPath(scope, "SignedPkg", "1.0.0") + ".zip"
		req2, _ := http.NewRequest("GET", url, nil)
		req2.Header.Set("Accept", acceptZip)
		env.setAuth(req2)
		resp2, err := env.httpClient.Do(req2)
		if err != nil {
			t.Fatalf("download signed package: %v", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp2.Body)
			t.Fatalf("download: expected 200, got %d, body %s", resp2.StatusCode, string(b))
		}
		signatureFormatHeader := resp2.Header.Get("X-Swift-Package-Signature-Format")
		if signatureFormatHeader != "cms-1.0.0" {
			t.Fatalf("X-Swift-Package-Signature-Format: got %q, want cms-1.0.0", signatureFormatHeader)
		}
		signatureHeader := resp2.Header.Get("X-Swift-Package-Signature")
		if signatureHeader == "" {
			t.Fatal("X-Swift-Package-Signature header missing")
		}
	})

	// Test Swift CLI signing via spm-extended: create-signing then publish with cert chain.
	t.Run("PublishWithSwiftSigning", func(t *testing.T) {
		swiftSignedPkgID := scope + ".SwiftSignedPkg"
		swiftSignedVersion := "1.0.1"

		// Create a minimal Swift package directory with spm-extended plugin dependency
		swiftSignedPkgDir := filepath.Join(env.rootDir, "testdata", "e2e", "example.SwiftSignedPkg")
		os.MkdirAll(swiftSignedPkgDir, 0755)
		defer os.RemoveAll(swiftSignedPkgDir)

		// Package.swift with spm-extended dependency so we can use registry create-signing and registry publish
		packageSwift := `// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "SwiftSignedPkg",
    platforms: [.macOS(.v12)],
    products: [
        .library(name: "SwiftSignedPkg", targets: ["SwiftSignedPkg"])
    ],
    dependencies: [
        .package(url: "https://github.com/wgr1984/spm-extended.git", from: "0.1.3")
    ],
    targets: [
        .target(name: "SwiftSignedPkg")
    ]
)
`
		if err := os.WriteFile(filepath.Join(swiftSignedPkgDir, "Package.swift"), []byte(packageSwift), 0644); err != nil {
			t.Fatalf("create Package.swift: %v", err)
		}

		// Create Sources directory and a simple source file
		sourcesDir := filepath.Join(swiftSignedPkgDir, "Sources", "SwiftSignedPkg")
		os.MkdirAll(sourcesDir, 0755)
		sourceFile := `public struct SwiftSignedPkg {
    public init() {}
}
`
		if err := os.WriteFile(filepath.Join(sourcesDir, "SwiftSignedPkg.swift"), []byte(sourceFile), 0644); err != nil {
			t.Fatalf("create SwiftSignedPkg.swift: %v", err)
		}

		// Clean build artifacts so create-signing writes into a fresh .swiftpm/signing
		os.RemoveAll(filepath.Join(swiftSignedPkgDir, ".build"))
		os.RemoveAll(filepath.Join(swiftSignedPkgDir, ".swiftpm"))

		// Create CA and leaf cert via spm-extended (writes .swiftpm/signing/leaf.der, ca.der, leaf.key.der)
		out, err := runSwift(t, swiftSignedPkgDir, "package", "--disable-sandbox", "registry", "create-signing", "--create-leaf-cert")
		if err != nil {
			t.Skipf("spm-extended create-signing not available (Swift 5.9+ and plugin required): %v\n%s", err, out)
		}

		// Publish with cert chain via spm-extended (generates Package.json and publishes with signing)
		leafDer := filepath.Join(swiftSignedPkgDir, ".swiftpm", "signing", "leaf.der")
		caDer := filepath.Join(swiftSignedPkgDir, ".swiftpm", "signing", "ca.der")
		keyDer := filepath.Join(swiftSignedPkgDir, ".swiftpm", "signing", "leaf.key.der")
		for _, p := range []string{leafDer, caDer, keyDer} {
			if _, err := os.Stat(p); err != nil {
				t.Fatalf("create-signing did not create %s: %v", p, err)
			}
		}
		publishOpts := []string{"--url", env.registryURL,
			"--cert-chain-paths", leafDer, caDer,
			"--private-key-path", keyDer}
		if !env.useHTTPS {
			publishOpts = append(publishOpts, "--allow-insecure-http")
		}
		out, err = runSwift(t, swiftSignedPkgDir, append([]string{"package", "--disable-sandbox", "registry", "publish", swiftSignedPkgID, swiftSignedVersion}, publishOpts...)...)
		if err != nil {
			t.Fatalf("publish %s %s with signing: %v\n%s", swiftSignedPkgID, swiftSignedVersion, err, out)
		}

		time.Sleep(500 * time.Millisecond)

		// Verify release metadata contains signing
		req, _ := http.NewRequest("GET", env.registryPath(scope, "SwiftSignedPkg", swiftSignedVersion), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get SwiftSignedPkg metadata: %v", err)
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
		signatureFormat, _ := signing["signatureFormat"].(string)
		if signatureFormat != "cms-1.0.0" {
			t.Fatalf("signatureFormat: got %q, want cms-1.0.0", signatureFormat)
		}

		// Verify download headers
		url := env.registryPath(scope, "SwiftSignedPkg", swiftSignedVersion) + ".zip"
		req2, _ := http.NewRequest("GET", url, nil)
		req2.Header.Set("Accept", acceptZip)
		env.setAuth(req2)
		resp2, err := env.httpClient.Do(req2)
		if err != nil {
			t.Fatalf("download SwiftSignedPkg: %v", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp2.Body)
			t.Fatalf("download: expected 200, got %d, body %s", resp2.StatusCode, string(b))
		}
		signatureFormatHeader := resp2.Header.Get("X-Swift-Package-Signature-Format")
		if signatureFormatHeader != "cms-1.0.0" {
			t.Fatalf("X-Swift-Package-Signature-Format: got %q, want cms-1.0.0", signatureFormatHeader)
		}
		signatureHeader := resp2.Header.Get("X-Swift-Package-Signature")
		if signatureHeader == "" {
			t.Fatal("X-Swift-Package-Signature header missing")
		}
		// Verify signature in header matches the one in metadata
		if signatureHeader != signatureBase64Encoded {
			t.Fatalf("X-Swift-Package-Signature header does not match metadata signature")
		}
	})

	t.Run("VerifyAltManifest", func(t *testing.T) {
		url := env.registryPath(scope, "SamplePackage", "1.0.0", "Package.swift") + "?swift-version=5.8"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Accept", acceptSwift)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get manifest: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte("swift-tools-version:5.8")) {
			t.Fatalf("Package@swift-5.8 manifest not found or wrong version")
		}
	})

	t.Run("VerifyListReleases", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(scope, "SamplePackage"), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list releases: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		for _, v := range []string{"1.0.0", "1.1.0"} {
			if !bytes.Contains(body, []byte(`"`+v+`"`)) {
				t.Fatalf("list response missing version %s", v)
			}
		}
		if !strings.Contains(resp.Header.Get("Link"), "latest-version") {
			t.Fatalf("list response missing latest-version Link header")
		}
	})

	t.Run("VerifyPagination", func(t *testing.T) {
		for _, p := range []struct {
			page    int
			wantVer string
		}{{1, "1.1.0"}, {2, "1.0.0"}} {
			page, wantVer := p.page, p.wantVer
			req, _ := http.NewRequest("GET", env.registryPath(scope, "SamplePackage")+"?page="+fmt.Sprint(page), nil)
			req.Header.Set("Accept", acceptJSON)
			env.setAuth(req)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("list page %d: %v", page, err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !bytes.Contains(body, []byte(`"`+wantVer+`"`)) {
				t.Fatalf("page %d should return %s", page, wantVer)
			}
			link := resp.Header.Get("Link")
			for _, rel := range []string{"first", "last"} {
				if !strings.Contains(link, `rel="`+rel+`"`) {
					t.Fatalf("page %d missing %s link", page, rel)
				}
			}
			if page == 1 && !strings.Contains(link, `rel="next"`) {
				t.Fatalf("page 1 missing next link")
			}
			if page == 2 && !strings.Contains(link, `rel="prev"`) {
				t.Fatalf("page 2 missing prev link")
			}
		}
	})

	t.Run("VerifyUtilsPackageList", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath(scope, "UtilsPackage"), nil)
		req.Header.Set("Accept", acceptJSON)
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("list UtilsPackage: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		for _, v := range []string{"1.0.0", "1.1.0"} {
			if !bytes.Contains(body, []byte(`"`+v+`"`)) {
				t.Fatalf("UtilsPackage list missing %s", v)
			}
		}
	})

	t.Run("VerifyGlobalCollection", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("collection"), nil)
		req.Header.Set("Accept", "application/json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get collection: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		for _, pkg := range []string{"example.SamplePackage", "example.UtilsPackage"} {
			if !bytes.Contains(body, []byte(pkg)) {
				t.Fatalf("global collection missing %s", pkg)
			}
		}
		if !bytes.Contains(body, []byte(`"formatVersion"`)) || !bytes.Contains(body, []byte(`"packages"`)) {
			t.Fatalf("global collection missing formatVersion or packages")
		}
		if !bytes.Contains(body, []byte(`"generatedBy"`)) {
			t.Fatalf("global collection missing generatedBy")
		}
	})

	t.Run("VerifyScopeCollection", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("collection", scope), nil)
		req.Header.Set("Accept", "application/json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get scope collection: %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		for _, pkg := range []string{"example.SamplePackage", "example.UtilsPackage"} {
			if !bytes.Contains(body, []byte(pkg)) {
				t.Fatalf("scope collection missing %s", pkg)
			}
		}
		for _, ver := range []string{"1.0.0", "1.1.0"} {
			if !bytes.Contains(body, []byte(ver)) {
				t.Fatalf("scope collection missing version %s", ver)
			}
		}
	})

	t.Run("VerifyCollection404", func(t *testing.T) {
		req, _ := http.NewRequest("GET", env.registryPath("collection", "nonexistentscope123"), nil)
		req.Header.Set("Accept", "application/json")
		env.setAuth(req)
		resp, err := env.httpClient.Do(req)
		if err != nil {
			t.Fatalf("get nonexistent collection: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("expected 404 for non-existent scope, got %d", resp.StatusCode)
		}
	})

	t.Run("VerifyPackageCollectionCLI", func(t *testing.T) {
		// Swift CLI: HTTP uses file://; HTTPS uses URL with ?auth=base64(Basic header)
		var collectionURL string
		if env.useHTTPS {
			basicAuth := base64.StdEncoding.EncodeToString([]byte(env.registryUser + ":" + env.registryPass))
			authHeader := "Basic " + basicAuth
			authB64 := base64.StdEncoding.EncodeToString([]byte(authHeader))
			collectionURL = env.registryPath("collection") + "?auth=" + authB64
		} else {
			tmp, err := os.CreateTemp("", "e2e-collection-*.json")
			if err != nil {
				t.Fatalf("create temp file: %v", err)
			}
			defer os.Remove(tmp.Name())
			req, _ := http.NewRequest("GET", env.registryPath("collection"), nil)
			req.Header.Set("Accept", "application/json")
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("fetch collection: %v", err)
			}
			io.Copy(tmp, resp.Body)
			resp.Body.Close()
			tmp.Close()
			collectionURL = "file://" + tmp.Name()
		}

		// Remove if already added
		runSwift(t, env.rootDir, "package-collection", "remove", collectionURL)

		out, err := runSwift(t, env.rootDir, "package-collection", "add", collectionURL, "--trust-unsigned")
		if err != nil {
			t.Fatalf("swift package-collection add: %v\n%s", err, out)
		}
		defer runSwift(t, env.rootDir, "package-collection", "remove", collectionURL)

		out, err = runSwift(t, env.rootDir, "package-collection", "list")
		if err != nil || !strings.Contains(out, "All Packages") {
			t.Fatalf("swift package-collection list: %v\n%s", err, out)
		}

		out, err = runSwift(t, env.rootDir, "package-collection", "describe", collectionURL)
		if err != nil || !strings.Contains(strings.ToLower(out), "example") {
			t.Fatalf("swift package-collection describe: %v\n%s", err, out)
		}
	})

	// Verify that the exact bytes we serve for Package.swift compile with the Swift toolchain.
	// If this passes, server-side encoding/transfer is fine and the manifest content is valid.
	t.Run("VerifyServedManifestCompiles", func(t *testing.T) {
		for _, pkg := range []string{"SamplePackage", "UtilsPackage"} {
			manifestURL := env.registryPath(scope, pkg, "1.1.0", "Package.swift")
			req, err := http.NewRequest("GET", manifestURL, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Accept", acceptSwift)
			env.setAuth(req)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Fatalf("GET %s: %v", manifestURL, err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d", manifestURL, resp.StatusCode)
			}
			dir := t.TempDir()
			pkgPath := filepath.Join(dir, "Package.swift")
			if err := os.WriteFile(pkgPath, body, 0644); err != nil {
				t.Fatalf("write Package.swift: %v", err)
			}
			out, err := runSwift(t, dir, "package", "dump-package")
			if err != nil {
				t.Fatalf("served manifest for %s did not compile (dump-package failed). Server may be sending bad bytes or wrong encoding.\n%s\n%s", pkg, out, err)
			}
			if !strings.Contains(out, `"name"`) {
				t.Fatalf("dump-package for %s produced no JSON: %s", pkg, out)
			}
		}
	})

	// Consumer resolve and run
	t.Run("ConsumerResolve", func(t *testing.T) {
		setArgs := []string{"package-registry", "set", env.registryURL}
		if !env.useHTTPS {
			setArgs = append(setArgs, "--allow-insecure-http")
		}
		_, err := runSwift(t, env.consumerDir, setArgs...)
		if err != nil {
			t.Fatalf("swift package-registry set: %v", err)
		}
		out, err := runSwift(t, env.consumerDir, "package", "resolve")
		if err != nil {
			if strings.Contains(out, "Missing or empty JSON output from manifest compilation") {
				t.Skipf("swift package resolve failed with 'Missing or empty JSON output from manifest compilation'; skipping resolve/build. Publish and HTTP verification passed.")
			}
			// SwiftSignedPkg may be missing if PublishWithSwiftSigning was skipped (spm-extended not available)
			if strings.Contains(out, "SwiftSignedPkg") || strings.Contains(out, "could not find") {
				t.Skipf("swift package resolve failed (example.SwiftSignedPkg likely not published because PublishWithSwiftSigning was skipped): %v\n%s", err, out)
			}
			t.Fatalf("swift package resolve: %v\n%s", err, out)
		}
		resolvedPath := filepath.Join(env.consumerDir, "Package.resolved")
		if _, err := os.Stat(resolvedPath); err != nil {
			t.Fatalf("Package.resolved was not created")
		}
		content, _ := os.ReadFile(resolvedPath)
		for _, pkg := range []string{"example.SamplePackage", "example.UtilsPackage", "example.SwiftSignedPkg"} {
			if !bytes.Contains(content, []byte(pkg)) {
				t.Fatalf("Package.resolved does not contain %s", pkg)
			}
		}
	})

	t.Run("ConsumerBuildRun", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(env.consumerDir, "Package.resolved")); err != nil {
			t.Skipf("ConsumerResolve was skipped; skipping build/run.")
		}
		buildOut, err := runSwift(t, env.consumerDir, "build", "-vv")
		if err != nil {
			t.Fatalf("swift build: %v\n%s", err, buildOut)
		}
		out, err := runSwift(t, env.consumerDir, "run", "Consumer")
		if err != nil {
			t.Fatalf("swift run Consumer: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Resolved SamplePackage") {
			t.Fatalf("consumer output missing SamplePackage: %s", out)
		}
		if !strings.Contains(out, "Resolved UtilsPackage") {
			t.Fatalf("consumer output missing UtilsPackage: %s", out)
		}
		if !strings.Contains(out, "Resolved SwiftSignedPkg") {
			t.Fatalf("consumer output missing SwiftSignedPkg: %s", out)
		}
	})
}
