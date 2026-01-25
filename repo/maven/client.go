package maven

import (
	"OpenSPMRegistry/config"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// client handles HTTP operations with Maven repositories
type client struct {
	httpClient *http.Client
	baseURL    string
	config     config.MavenConfig
}

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
// or builds it from configured credentials (config mode)
func (c *client) getAuthHeader(ctx context.Context) string {
	// Check context first (passthrough mode)
	if ctxAuth := ctx.Value("Authorization"); ctxAuth != nil {
		if authHeader, ok := ctxAuth.(string); ok && authHeader != "" {
			return authHeader
		}
	}

	// Fall back to configured credentials (config mode)
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
	fullURL, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
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

	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
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
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT request failed with status %d: %s, body: %s", resp.StatusCode, resp.Status, string(bodyBytes))
	}

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("PUT request successful", "path", path, "status", resp.StatusCode)
	}

	return nil
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
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("DELETE request failed with status %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}
