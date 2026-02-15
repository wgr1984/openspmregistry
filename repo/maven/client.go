package maven

import (
	"OpenSPMRegistry/config"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// client handles HTTP operations with Maven repositories
type client struct {
	httpClient *http.Client
	baseURL    string
	config     config.MavenConfig
}

// spmRegistryIndexResponse is the JSON structure of .spm-registry/index.json.
type spmRegistryIndexResponse struct {
	Scopes   []string            `json:"scopes"`
	Packages map[string][]string `json:"packages,omitempty"`
}

// spmRegistryIndexPath is the well-known path for the SPM registry scope index (relative to repo base URL).
// Uses Maven 2 layout (groupId/artifactId/version/file) so strict Maven repos (e.g. Nexus) accept PUT/GET.
const spmRegistryIndexPath = "com/spm/registry/index/1/index-1.json"

// newClient creates a new Maven HTTP client
func newClient(cfg config.MavenConfig) (*client, error) {
	// Default timeout: 30 seconds (configurable via config.yml maven.timeout)
	timeout := 30 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}

	return &client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		config:  cfg,
	}, nil
}

// getAuthHeader retrieves the Authorization header from context (passthrough mode)
// or builds it from configured credentials (config mode).
// When authMode is "config", always use configured credentials for the Maven backend;
// passthrough is only used when authMode is "passthrough".
func (c *client) getAuthHeader(ctx context.Context) string {
	if c.config.AuthMode == "passthrough" {
		if ctxAuth := ctx.Value(config.AuthHeaderContextKey); ctxAuth != nil {
			if authHeader, ok := ctxAuth.(string); ok && authHeader != "" {
				return authHeader
			}
		}
		return ""
	}

	if c.config.AuthMode == "config" || (c.config.AuthMode == "" && c.config.Username != "") {
		return c.buildBasicAuth(c.config.Username, c.config.Password)
	}

	return ""
}

// buildBasicAuth creates a Basic Auth header from username and password
func (c *client) buildBasicAuth(username, password string) string {
	auth := username + ":" + password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// makeRequest creates an HTTP request with authentication
func (c *client) makeRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	var fullURL string
	if path == "" {
		// Nexus requires a path segment after the repo key (e.g. .../repository/private/); use trailing slash
		fullURL = strings.TrimSuffix(c.baseURL, "/") + "/"
	} else {
		var err error
		fullURL, err = url.JoinPath(c.baseURL, path)
		if err != nil {
			return nil, fmt.Errorf("failed to build URL: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	if authHeader := c.getAuthHeader(ctx); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	return req, nil
}

// doRequest executes an HTTP request and returns the response
func (c *client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := c.makeRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Caller must close resp.Body; do not close here so GET callers can read the body.
	if resp.StatusCode >= http.StatusBadRequest {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	return resp, nil
}

// HEAD performs a HEAD request to check existence and get metadata
func (c *client) HEAD(ctx context.Context, path string) (*http.Response, error) {
	req, err := c.makeRequest(ctx, "HEAD", path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD request failed: %w", err)
	}

	// Don't close body here, caller should handle it
	return resp, nil
}

// GET performs a GET request to download artifacts
func (c *client) GET(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// getSPMRegistryIndexFull fetches .spm-registry/index.json and returns the full decoded index.
// On 404 or non-200 status returns (nil, error). Missing or null scopes/packages are normalized to empty.
func (c *client) getSPMRegistryIndexFull(ctx context.Context) (*spmRegistryIndexResponse, error) {
	resp, err := c.GET(ctx, spmRegistryIndexPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	var index spmRegistryIndexResponse
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to decode index.json: %w", err)
	}
	if index.Scopes == nil {
		index.Scopes = []string{}
	}
	if index.Packages == nil {
		index.Packages = make(map[string][]string)
	}
	return &index, nil
}

// getSPMRegistryIndex fetches .spm-registry/index.json and returns the list of scopes, sorted.
// On 404 or non-200 status it returns (nil, error). On 200 it decodes JSON and returns a sorted copy of scopes.
func (c *client) getSPMRegistryIndex(ctx context.Context) ([]string, error) {
	index, err := c.getSPMRegistryIndexFull(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(index.Scopes))
	copy(out, index.Scopes)
	sort.Strings(out)
	return out, nil
}

// PUT performs a PUT request to upload artifacts
func (c *client) PUT(ctx context.Context, path string, body io.Reader, contentType string) error {
	req, err := c.makeRequest(ctx, "PUT", path, body)
	if err != nil {
		return err
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PUT request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return fmt.Errorf("PUT request failed with status %d: %s, body: %s", resp.StatusCode, resp.Status, string(bodyBytes))
	}

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("PUT request successful", "path", path, "status", resp.StatusCode)
	}

	return resp.Body.Close()
}

// DELETE performs a DELETE request to remove artifacts
func (c *client) DELETE(ctx context.Context, path string) error {
	req, err := c.makeRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE request failed: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("DELETE request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	return err
}
