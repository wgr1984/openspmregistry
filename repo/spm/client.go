package spm

import (
	"OpenSPMRegistry/config"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// client handles HTTP operations with an upstream Swift Package Registry.
type client struct {
	httpClient *http.Client
	baseURL    string
	cfg        config.SPMConfig
}

// HTTPStatusError carries the HTTP status code for a failed request.
type HTTPStatusError struct {
	StatusCode int
}

// ErrHTTPStatus is returned when the server responds with status >= 400.
var ErrHTTPStatus = errors.New("http request failed with error status")

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("request failed with status %d", e.StatusCode)
}

func (e *HTTPStatusError) Is(target error) bool {
	return target == ErrHTTPStatus
}

// newClient creates a new SPM upstream HTTP client.
func newClient(cfg config.SPMConfig) (*client, error) {
	timeout := 30 * time.Second
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}
	return &client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
		cfg:        cfg,
	}, nil
}

// getAuthHeader returns the Authorization header value to send to the upstream.
// In passthrough mode the client's own header (stored in ctx) is forwarded.
// In config mode the configured username/password are used.
func (c *client) getAuthHeader(ctx context.Context) string {
	if c.cfg.AuthMode == "passthrough" {
		if ctxAuth := ctx.Value(config.AuthHeaderContextKey); ctxAuth != nil {
			if authHeader, ok := ctxAuth.(string); ok && authHeader != "" {
				return authHeader
			}
		}
		return ""
	}
	if c.cfg.AuthMode == "config" || (c.cfg.AuthMode == "" && c.cfg.Username != "") {
		auth := c.cfg.Username + ":" + c.cfg.Password
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	return ""
}

// makeRequest builds an HTTP request for the upstream registry.
func (c *client) makeRequest(ctx context.Context, method, path, query, accept string) (*http.Request, error) {
	var fullURL string
	var err error
	if path == "" {
		fullURL = c.baseURL + "/"
	} else {
		fullURL, err = url.JoinPath(c.baseURL, path)
		if err != nil {
			return nil, fmt.Errorf("failed to build URL: %w", err)
		}
	}
	if query != "" {
		fullURL += "?" + query
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if authHeader := c.getAuthHeader(ctx); authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	} else {
		req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")
	}
	return req, nil
}

// HEAD performs a HEAD request against the upstream registry.
// The caller must close the response body.
func (c *client) HEAD(ctx context.Context, path, query string) (*http.Response, error) {
	req, err := c.makeRequest(ctx, http.MethodHead, path, query, "")
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HEAD request failed: %w", err)
	}
	return resp, nil
}

// GET performs a GET request and returns the response.
// On HTTP 4xx/5xx the body is closed and an HTTPStatusError is returned.
// On success the caller must close the response body.
func (c *client) GET(ctx context.Context, path, query, accept string) (*http.Response, error) {
	req, err := c.makeRequest(ctx, http.MethodGet, path, query, accept)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET request failed: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
	}
	return resp, nil
}
