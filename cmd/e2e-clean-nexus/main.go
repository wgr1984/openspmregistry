// e2e-clean-nexus deletes example scope E2E package components (SamplePackage, UtilsPackage)
// from the Nexus private repo. Used by E2E tests to ensure a clean state before publish.
//
// Environment: NEXUS_URL, MAVEN_REPO_NAME, MAVEN_REPO_USERNAME, MAVEN_REPO_PASSWORD, E2E_PACKAGES.
// Optional: password from .nexus-test-password (relative to repo root) overrides MAVEN_REPO_PASSWORD.
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

func main() {
	nexusURL := getEnv("NEXUS_URL", "http://localhost:8081")
	repo := getEnv("MAVEN_REPO_NAME", "private")
	user := getEnv("MAVEN_REPO_USERNAME", "admin")
	pass := getEnv("MAVEN_REPO_PASSWORD", "admin123")
	packagesStr := getEnv("E2E_PACKAGES", "SamplePackage UtilsPackage")

	if f := findPasswordFile(); f != "" {
		b, err := os.ReadFile(f)
		if err == nil {
			pass = strings.TrimSpace(string(b))
		}
	}

	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	client := &http.Client{}
	packages := strings.Fields(packagesStr)
	totalDeleted := 0

	for _, pkgName := range packages {
		n, err := cleanPackage(client, nexusURL, repo, pkgName, auth)
		if err != nil {
			slog.Error("clean failed", "package", pkgName, "err", err)
			os.Exit(1)
		}
		totalDeleted += n
		if n > 0 {
			slog.Info("cleaned components", "package", "example."+pkgName, "count", n)
		}
	}

	if totalDeleted > 0 {
		slog.Info("total cleaned", "count", totalDeleted)
	}
}

func findPasswordFile() string {
	if p := os.Getenv("NEXUS_TEST_PASSWORD_FILE"); p != "" {
		return p
	}
	cwd, _ := os.Getwd()
	if cwd == "" {
		return ""
	}
	for _, root := range []string{cwd, filepath.Dir(cwd)} {
		candidate := filepath.Join(root, ".nexus-test-password")
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

type searchResponse struct {
	Items             []struct{ ID string } `json:"items"`
	ContinuationToken string                `json:"continuationToken"`
}

func cleanPackage(client *http.Client, nexusURL, repo, pkgName, auth string) (int, error) {
	baseURL := fmt.Sprintf("%s/service/rest/v1/search?repository=%s&group=example&name=%s",
		nexusURL, repo, pkgName)
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
		resp.Body.Close()
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
				r.Body.Close()
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
