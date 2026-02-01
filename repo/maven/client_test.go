package maven

import (
	"OpenSPMRegistry/config"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_newClient_ValidConfig_ReturnsClient(t *testing.T) {
	cfg := config.MavenConfig{
		BaseURL: "https://repo.example.com",
		Timeout: 60,
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Errorf("expected client, got nil")
		return
	}
	if c.baseURL != "https://repo.example.com" {
		t.Errorf("expected baseURL 'https://repo.example.com', got '%s'", c.baseURL)
	}
	if c.httpClient.Timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", c.httpClient.Timeout)
	}
}

func Test_newClient_DefaultTimeout_ReturnsClientWithDefaultTimeout(t *testing.T) {
	cfg := config.MavenConfig{
		BaseURL: "https://repo.example.com",
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", c.httpClient.Timeout)
	}
}

func Test_newClient_BaseURLWithTrailingSlash_TrimsSlash(t *testing.T) {
	cfg := config.MavenConfig{
		BaseURL: "https://repo.example.com/",
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.baseURL != "https://repo.example.com" {
		t.Errorf("expected baseURL without trailing slash, got '%s'", c.baseURL)
	}
}

func Test_buildBasicAuth_ValidCredentials_ReturnsAuthHeader(t *testing.T) {
	c := &client{}
	result := c.buildBasicAuth("user", "pass")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_getAuthHeader_ContextAuth_ReturnsContextAuth(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	ctx := context.WithValue(context.Background(), config.AuthHeaderContextKey, "Bearer token123")
	result := c.getAuthHeader(ctx)
	if result != "Bearer token123" {
		t.Errorf("expected 'Bearer token123', got '%s'", result)
	}
}

func Test_getAuthHeader_ConfigMode_ReturnsConfigAuth(t *testing.T) {
	cfg := config.MavenConfig{
		AuthMode: "config",
		Username: "user",
		Password: "pass",
	}
	c, _ := newClient(cfg)
	ctx := context.Background()
	result := c.getAuthHeader(ctx)
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_getAuthHeader_UsernameWithoutAuthMode_ReturnsConfigAuth(t *testing.T) {
	cfg := config.MavenConfig{
		Username: "user",
		Password: "pass",
	}
	c, _ := newClient(cfg)
	ctx := context.Background()
	result := c.getAuthHeader(ctx)
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_getAuthHeader_NoAuth_ReturnsEmpty(t *testing.T) {
	cfg := config.MavenConfig{}
	c, _ := newClient(cfg)
	ctx := context.Background()
	result := c.getAuthHeader(ctx)
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func Test_HEAD_Success_ReturnsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := c.HEAD(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func Test_GET_Success_ReturnsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test data"))
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := c.GET(context.Background(), "test/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func Test_GET_ErrorStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = c.GET(context.Background(), "test/path")
	if err == nil {
		t.Errorf("expected error for 404, got nil")
	}
}

func Test_PUT_Success_ReturnsNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	data := strings.NewReader("test data")
	err = c.PUT(context.Background(), "test/path", data, "application/zip")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_PUT_ErrorStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	data := strings.NewReader("test data")
	err = c.PUT(context.Background(), "test/path", data, "application/zip")
	if err == nil {
		t.Errorf("expected error for 403, got nil")
	}
}

func Test_DELETE_Success_ReturnsNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = c.DELETE(context.Background(), "test/path")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_DELETE_NotFound_ReturnsNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = c.DELETE(context.Background(), "test/path")
	if err != nil {
		t.Errorf("expected no error for 404, got %v", err)
	}
}

func Test_DELETE_ErrorStatus_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	err = c.DELETE(context.Background(), "test/path")
	if err == nil {
		t.Errorf("expected error for 403, got nil")
	}
}

func Test_makeRequest_WithAuth_IncludesAuthHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.MavenConfig{
		BaseURL:  server.URL,
		AuthMode: "config",
		Username: "user",
		Password: "pass",
	}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	req, err := c.makeRequest(context.Background(), "GET", "test/path", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Errorf("expected Authorization header, got empty")
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Basic auth, got '%s'", auth)
	}
}
