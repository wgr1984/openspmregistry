//go:build e2e
// +build e2e

// Package e2e provides end-to-end tests for the Swift package registry.
// Run with: go test -tags=e2e -v ./e2e/...
// Prerequisites: Nexus running (make test-integration-up), Swift toolchain installed.
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
)

const (
	defaultRegistryURL = "http://127.0.0.1:8082"
	nexusURL          = "http://localhost:8081"
	nexusRepo         = "private"
	scope             = "example"
	acceptJSON        = "application/vnd.swift.registry.v1+json"
	acceptSwift       = "application/vnd.swift.registry.v1+swift"
	serverReadyWait   = 30 * time.Second
)

// e2eEnv holds paths and config for the E2E test.
type e2eEnv struct {
	rootDir      string
	configPath   string
	registryURL  string
	useHTTPS     bool
	registryUser string
	registryPass string
	consumerDir  string
	samplePkgDir string
	utilsPkgDir  string
	nexusUser    string
	nexusPass    string
	httpClient   *http.Client
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

	configPath := filepath.Join(root, "config.e2e.yml")
	if useHTTPS {
		configPath = filepath.Join(root, "config.e2e.https.yml")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("missing %s: %v", configPath, err)
	}

	nexusUser := os.Getenv("MAVEN_REPO_USERNAME")
	if nexusUser == "" {
		nexusUser = "admin"
	}
	nexusPass := os.Getenv("MAVEN_REPO_PASSWORD")
	if nexusPass == "" {
		if b, err := os.ReadFile(filepath.Join(root, ".nexus-test-password")); err == nil {
			nexusPass = strings.TrimSpace(string(b))
		} else {
			nexusPass = "admin123"
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
		rootDir:      root,
		configPath:   configPath,
		registryURL:  registryURL,
		useHTTPS:     useHTTPS,
		registryUser: registryUser,
		registryPass: registryPass,
		consumerDir:  filepath.Join(root, "testdata", "e2e", "Consumer"),
		samplePkgDir: filepath.Join(root, "testdata", "e2e", "example.SamplePackage"),
		utilsPkgDir:  filepath.Join(root, "testdata", "e2e", "example.UtilsPackage"),
		nexusUser:    nexusUser,
		nexusPass:    nexusPass,
		httpClient:   httpClient,
	}
	return env
}

// cleanNexus removes example.SamplePackage and example.UtilsPackage from Nexus via REST API.
func cleanNexus(t *testing.T, env *e2eEnv) {
	t.Helper()
	for _, pkgName := range []string{"SamplePackage", "UtilsPackage"} {
		baseURL := fmt.Sprintf("%s/service/rest/v1/search?repository=%s&group=example&name=%s",
			nexusURL, nexusRepo, pkgName)
		auth := base64.StdEncoding.EncodeToString([]byte(env.nexusUser + ":" + env.nexusPass))
		token := ""
		for {
			url := baseURL
			if token != "" {
				url += "&continuationToken=" + token
			}
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				t.Logf("cleanNexus: create request: %v", err)
				return
			}
			req.Header.Set("Authorization", "Basic "+auth)
			resp, err := env.httpClient.Do(req)
			if err != nil {
				t.Logf("cleanNexus: request failed: %v", err)
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				return
			}
			var data struct {
				Items              []struct{ ID string `json:"id"` }
				ContinuationToken  string `json:"continuationToken"`
			}
			if err := json.Unmarshal(body, &data); err != nil {
				t.Logf("cleanNexus: parse response: %v", err)
				return
			}
			for _, item := range data.Items {
				delURL := fmt.Sprintf("%s/service/rest/v1/components/%s", nexusURL, item.ID)
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
}

// waitForNexus checks that Nexus is reachable.
func waitForNexus(t *testing.T, env *e2eEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", nexusURL+"/service/rest/v1/status", nil)
	resp, err := env.httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Skipf("Nexus not reachable at %s. Start with: make test-integration-up", nexusURL)
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
	waitForNexus(t, env)

	if !swiftAvailable() {
		t.Skip("Swift toolchain not found. Install Swift to run this test.")
	}

	// Clean state (matches script: Setup section)
	os.Remove(filepath.Join(env.consumerDir, "Package.resolved"))
	os.RemoveAll(filepath.Join(env.consumerDir, ".build"))
	cleanNexus(t, env)

	// Purge Swift PM cache (script lines 104-107; ignore errors)
	pc := exec.Command("swift", "package", "purge-cache")
	pc.Dir = env.consumerDir
	pc.Run()
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), "Library", "Caches", "org.swift.swiftpm"))
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), "Library", "org.swift.swiftpm"))
	os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".cache", "org.swift.swiftpm"))

	// Free port 8082 in case previous run didn't exit cleanly (script lines 136-140)
	if _, err := exec.LookPath("lsof"); err == nil {
		exec.Command("sh", "-c", "lsof -ti :8082 | xargs kill -9 2>/dev/null || true").Run()
		time.Sleep(time.Second)
	}

	// Build and start server
	binaryPath := filepath.Join(env.rootDir, "openspmregistry.e2e")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "main.go")
	buildCmd.Dir = env.rootDir
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("build server: %v", err)
	}
	defer os.Remove(binaryPath)

	cmd := exec.Command(binaryPath, "-config", env.configPath, "-v")
	cmd.Dir = env.rootDir
	var serverLog bytes.Buffer
	cmd.Stdout = &serverLog
	cmd.Stderr = &serverLog
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		if t.Failed() {
			t.Logf("--- Server log ---\n%s", serverLog.String())
		}
	}()

	waitForRegistry(t, env)

	// HTTPS: ensure certs exist and login
	if env.useHTTPS {
		certsDir := filepath.Join(env.rootDir, "testdata", "e2e", "certs")
		if _, err := os.Stat(filepath.Join(certsDir, "server.crt")); err != nil {
			// Generate certs
			genCmd := exec.Command("bash", filepath.Join(env.rootDir, "scripts", "e2e-generate-certs.sh"))
			genCmd.Dir = env.rootDir
			if err := genCmd.Run(); err != nil {
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
			t.Fatalf("swift package resolve: %v\n%s", err, out)
		}
		resolvedPath := filepath.Join(env.consumerDir, "Package.resolved")
		if _, err := os.Stat(resolvedPath); err != nil {
			t.Fatalf("Package.resolved was not created")
		}
		content, _ := os.ReadFile(resolvedPath)
		for _, pkg := range []string{"example.SamplePackage", "example.UtilsPackage"} {
			if !bytes.Contains(content, []byte(pkg)) {
				t.Fatalf("Package.resolved does not contain %s", pkg)
			}
		}
	})

	t.Run("ConsumerBuildRun", func(t *testing.T) {
		if _, err := runSwift(t, env.consumerDir, "build"); err != nil {
			t.Fatalf("swift build: %v", err)
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
	})
}
