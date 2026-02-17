// nexus-bootstrap configures Nexus after startup: reads initial admin password (from container),
// creates Maven repo "private" via Script API, configures repo (strictContentTypeValidation=false,
// writePolicy=ALLOW), sets admin password to a fixed value, and optionally writes the password to a file.
// Idempotent: safe to run multiple times.
//
// Environment: NEXUS_URL, NEXUS_CONTAINER, NEXUS_REPO_KEY, NEXUS_TARGET_PASSWORD, NEXUS_TEST_PASSWORD_FILE.
//
// Usage: go run ./cmd/nexus-bootstrap
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func main() {
	nexusURL := getEnv("NEXUS_URL", "http://localhost:8081")
	container := getEnv("NEXUS_CONTAINER", "nexus-test")
	repoKey := getEnv("NEXUS_REPO_KEY", "private")
	targetPass := getEnv("NEXUS_TARGET_PASSWORD", "admin123")
	passwordFile := os.Getenv("NEXUS_TEST_PASSWORD_FILE")

	slog.Info("bootstrapping Nexus", "url", nexusURL)

	adminPass := getAdminPassword(container, targetPass)
	client := &http.Client{}

	scriptName := "maven-hosted-" + repoKey
	if err := uploadScript(client, nexusURL, "admin", adminPass, scriptName, fmt.Sprintf("repository.createMavenHosted('%s')", repoKey)); err != nil {
		slog.Error("upload create-repo script", "err", err)
		os.Exit(1)
	}
	slog.Info("script uploaded", "name", scriptName)

	if err := runScript(client, nexusURL, "admin", adminPass, scriptName); err != nil {
		slog.Warn("run create-repo script", "err", err)
	}
	slog.Info("repository created or already exists", "repo", repoKey)

	configureScriptName := "configure-repo-" + repoKey
	configureGroovy := fmt.Sprintf("def repo = repository.repositoryManager.get('%s'); if (repo != null) { def config = repo.configuration; def storage = config.attributes('storage'); storage.set('strictContentTypeValidation', false); storage.set('writePolicy', 'ALLOW'); repository.repositoryManager.update(config); return 'ok'; }; return 'repo not found';", repoKey)
	if err := uploadScript(client, nexusURL, "admin", adminPass, configureScriptName, configureGroovy); err != nil {
		slog.Warn("upload configure-repo script", "err", err)
	}
	if err := runScript(client, nexusURL, "admin", adminPass, configureScriptName); err != nil {
		slog.Warn("run configure-repo script", "err", err)
	} else {
		slog.Info("repo configured", "repo", repoKey)
	}

	effectivePass := adminPass
	if adminPass != targetPass {
		if err := changePassword(client, nexusURL, "admin", adminPass, targetPass); err != nil {
			slog.Warn("change admin password", "err", err)
		} else {
			effectivePass = targetPass
			slog.Info("admin password updated")
		}
	}

	if passwordFile != "" {
		if err := os.WriteFile(passwordFile, []byte(effectivePass), 0o600); err != nil {
			slog.Warn("write password file", "path", passwordFile, "err", err)
		} else {
			slog.Info("wrote effective password", "path", passwordFile)
		}
	}

	slog.Info("Nexus bootstrap done")
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getAdminPassword(container, targetPass string) string {
	cmd := exec.Command("docker", "exec", container, "cat", "/nexus-data/admin.password")
	out, err := cmd.Output()
	if err != nil {
		slog.Info("no admin.password file; using target password for API")
		return targetPass
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return targetPass
	}
	return s
}

func uploadScript(client *http.Client, baseURL, user, pass, name, content string) error {
	body := map[string]string{"name": name, "type": "groovy", "content": content}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/service/rest/v1/script", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case 200, 201, 204:
		slog.Info("script uploaded", "name", name)
		return nil
	case 400:
		slog.Info("script already exists", "name", name)
		return nil
	case 500:
		lower := strings.ToLower(string(b))
		if strings.Contains(lower, "duplicated") || strings.Contains(lower, "script_name_idx") || strings.Contains(lower, "duplicatedexception") {
			slog.Info("script already exists (500)", "name", name)
			return nil
		}
		return fmt.Errorf("upload script HTTP 500: %s", string(b))
	default:
		return fmt.Errorf("upload script HTTP %d: %s", resp.StatusCode, string(b))
	}
}

func runScript(client *http.Client, baseURL, user, pass, name string) error {
	url := baseURL + "/service/rest/v1/script/" + name + "/run"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 {
		return nil
	}
	s := string(b)
	if strings.Contains(strings.ToLower(s), "already exists") || strings.Contains(s, "Conflict") || strings.Contains(s, "409") {
		return nil
	}
	return fmt.Errorf("run script HTTP %d: %s", resp.StatusCode, s)
}

func changePassword(client *http.Client, baseURL, user, currentPass, newPass string) error {
	url := baseURL + "/service/rest/v1/security/users/admin/change-password"
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte(newPass)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, currentPass)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("change password HTTP %d: %s", resp.StatusCode, string(b))
}
