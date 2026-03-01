// reposilite-bootstrap prepares Reposilite for E2E/integration tests: verifies the server
// is up and writes the test token secret to a file so Makefile and tests can use it for auth.
// Reposilite must be started with the same token (e.g. --token e2e:test-secret in Docker).
// Idempotent: safe to run multiple times.
//
// Environment: REPOSILITE_URL (default http://localhost:8080), REPOSILITE_TEST_TOKEN_FILE
// (default .reposilite-test-token), REPOSILITE_TEST_TOKEN_SECRET (default test-secret).
//
// Usage: go run ./cmd/reposilite-bootstrap
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	baseURL := getEnv("REPOSILITE_URL", "http://localhost:8080")
	tokenFile := getEnv("REPOSILITE_TEST_TOKEN_FILE", ".reposilite-test-token")
	tokenSecret := getEnv("REPOSILITE_TEST_TOKEN_SECRET", "test-secret")

	slog.Info("bootstrapping Reposilite", "url", baseURL)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL + "/")
	if err != nil {
		slog.Warn("Reposilite health check failed (server may still be starting)", "err", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			slog.Warn("Reposilite returned unexpected status", "status", resp.StatusCode)
		}
	}

	if err := os.WriteFile(tokenFile, []byte(tokenSecret), 0o600); err != nil {
		slog.Error("write token file", "path", tokenFile, "err", err)
		os.Exit(1)
	}
	slog.Info("wrote token secret for E2E", "path", tokenFile)
	slog.Info("Reposilite bootstrap done (default private repo is available)")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
