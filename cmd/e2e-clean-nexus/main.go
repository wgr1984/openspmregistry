// e2e-clean-nexus deletes the entire "com" and "example" trees from the Maven test repo (Nexus or Reposilite).
// Use before E2E runs for a clean state.
//
// Nexus: uses REST API (server must be running). Env: MAVEN_PROVIDER=nexus, NEXUS_URL, MAVEN_REPO_NAME,
// MAVEN_REPO_USERNAME, MAVEN_REPO_PASSWORD. Optional: .nexus-test-password.
//
// Reposilite: removes reposilite-data/repositories/<repo>/com and .../example on disk (no server needed).
// Run from repo root or set REPOSILITE_DATA_DIR. Env: MAVEN_PROVIDER=reposilite, MAVEN_REPO_NAME.
// Optional: .reposilite-test-token not used for filesystem cleanup.
//
// Usage: go run ./cmd/e2e-clean-nexus
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type searchResponse struct {
	Items             []struct{ ID string } `json:"items"`
	ContinuationToken string                `json:"continuationToken"`
}

// e2eScopes are the top-level trees to remove (SPM scope "example" + registry index "com").
var e2eScopes = []string{"com", "example", "e2emaven", "e2esign"}

func main() {
	provider := getEnv("MAVEN_PROVIDER", "nexus")
	repo := getEnv("MAVEN_REPO_NAME", "private")

	switch provider {
	case "nexus":
		nexusURL := getEnv("NEXUS_URL", "http://localhost:8081")
		user := getEnv("MAVEN_REPO_USERNAME", "admin")
		pass := getEnv("MAVEN_REPO_PASSWORD", "admin123")
		if f := findPasswordFile(".nexus-test-password"); f != "" {
			if b, err := os.ReadFile(f); err == nil {
				pass = strings.TrimSpace(string(b))
			}
		}
		auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		client := &http.Client{}
		total := 0
		for _, group := range e2eScopes {
			n, err := cleanNexusGroup(client, nexusURL, repo, group, auth)
			if err != nil {
				slog.Error("clean failed", "group", group, "err", err)
				os.Exit(1)
			}
			total += n
			if n > 0 {
				slog.Info("removed", "group", group, "components", n)
			}
		}
		if total > 0 {
			slog.Info("total removed", "count", total)
		}
	case "reposilite":
		dataDir := findReposiliteDataDir(repo)
		if dataDir == "" {
			slog.Error("reposilite-data not found", "hint", "run from repo root or set REPOSILITE_DATA_DIR")
			os.Exit(1)
		}
		for _, scope := range e2eScopes {
			dir := filepath.Join(dataDir, "repositories", repo, scope)
			if err := os.RemoveAll(dir); err != nil {
				slog.Error("remove failed", "dir", dir, "err", err)
				os.Exit(1)
			}
			slog.Info("removed", "path", filepath.Join("repositories", repo, scope))
		}
	default:
		slog.Error("unknown MAVEN_PROVIDER", "provider", provider)
		os.Exit(1)
	}
}

func findReposiliteDataDir(repo string) string {
	if d := os.Getenv("REPOSILITE_DATA_DIR"); d != "" {
		marker := filepath.Join(d, "repositories", repo)
		if info, err := os.Stat(marker); err == nil && info.IsDir() {
			return d
		}
		return ""
	}
	cwd, _ := os.Getwd()
	if cwd == "" {
		return ""
	}
	for _, root := range []string{cwd, filepath.Dir(cwd)} {
		dataDir := filepath.Join(root, "reposilite-data")
		marker := filepath.Join(dataDir, "repositories", repo)
		if info, err := os.Stat(marker); err == nil && info.IsDir() {
			return dataDir
		}
	}
	return ""
}

func findPasswordFile(name string) string {
	if p := os.Getenv("NEXUS_TEST_PASSWORD_FILE"); p != "" && name == ".nexus-test-password" {
		return p
	}
	cwd, _ := os.Getwd()
	if cwd == "" {
		return ""
	}
	for _, root := range []string{cwd, filepath.Dir(cwd)} {
		candidate := filepath.Join(root, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// cleanNexusGroup deletes all components in the given group (e.g. "com" or "example") via Nexus REST API.
func cleanNexusGroup(client *http.Client, nexusURL, repo, group, auth string) (int, error) {
	baseURL := fmt.Sprintf("%s/service/rest/v1/search?repository=%s&group=%s", nexusURL, repo, group)
	deleted := 0
	token := ""

	for {
		url := baseURL
		if token != "" {
			url += "&continuationToken=" + token
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return deleted, err
		}
		req.Header.Set("Authorization", "Basic "+auth)
		resp, err := client.Do(req)
		if err != nil {
			return deleted, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return deleted, err
		}
		if resp.StatusCode == 404 || len(body) == 0 {
			break
		}
		if resp.StatusCode != 200 {
			return deleted, fmt.Errorf("nexus search failed: HTTP %d", resp.StatusCode)
		}
		var data searchResponse
		if err := json.Unmarshal(body, &data); err != nil {
			return deleted, fmt.Errorf("parse search response: %w", err)
		}
		for _, item := range data.Items {
			if item.ID == "" {
				continue
			}
			delURL := fmt.Sprintf("%s/service/rest/v1/components/%s", nexusURL, item.ID)
			delReq, _ := http.NewRequest(http.MethodDelete, delURL, nil)
			delReq.Header.Set("Authorization", "Basic "+auth)
			if r, err := client.Do(delReq); err == nil {
				_ = r.Body.Close()
				if r.StatusCode >= 200 && r.StatusCode < 300 {
					deleted++
				}
			}
		}
		token = data.ContinuationToken
		if token == "" {
			break
		}
	}
	return deleted, nil
}
