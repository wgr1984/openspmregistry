package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func Test_NewHeaderError_ReturnsCorrectErrorMessage(t *testing.T) {
	err := NewHeaderError("test error message")
	if err.Error() != "test error message" {
		t.Errorf("expected 'test error message', got %s", err.Error())
	}
}

func Test_NewHeaderError_SetsBadRequestStatusCode(t *testing.T) {
	err := NewHeaderError("test error message")
	if err.httpStatusCode != http.StatusBadRequest {
		t.Errorf("expected %d, got %d", http.StatusBadRequest, err.httpStatusCode)
	}
}

func Test_HeaderError_WriteResponse_SetsCorrectHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	err := NewHeaderError("test error message")
	err.writeResponse(w)

	if w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("expected 'application/problem+json', got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Content-Language") != "en" {
		t.Errorf("expected 'en', got %s", w.Header().Get("Content-Language"))
	}
}

func Test_HeaderError_WriteResponse_SetsCorrectStatusCode(t *testing.T) {
	w := httptest.NewRecorder()
	err := NewHeaderError("test error message")
	err.writeResponse(w)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func Test_HeaderError_WriteResponse_WritesErrorMessage(t *testing.T) {
	w := httptest.NewRecorder()
	err := NewHeaderError("test error message")
	err.writeResponse(w)

	var response responses.Error
	json.NewDecoder(w.Body).Decode(&response)
	if response.Detail != "test error message" {
		t.Errorf("expected 'test error message', got %s", response.Detail)
	}
}

func Test_CheckHeaders_ValidHeader_ReturnsNil(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")

	if err := checkHeaders(req); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func Test_CheckHeaders_InvalidVersion_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v2+json")

	err := checkHeaders(req)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "unsupported API version: 2" {
		t.Errorf("expected 'unsupported API version: 2', got %s", err.Error())
	}
}

func Test_CheckHeaders_UnsupportedMediaType_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+xml")

	err := checkHeaders(req)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "unsupported media type: xml" {
		t.Errorf("expected 'unsupported media type: xml', got %s", err.Error())
	}
}

func Test_CheckHeaders_MissingAcceptHeader_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	err := checkHeaders(req)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "missing Accept header" {
		t.Errorf("expected 'missing Accept header', got %s", err.Error())
	}
}

func Test_CheckHeadersEnforce_ValidHeaderWithEnforcedMediaType_ReturnsNil(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+json")

	if err := checkHeadersEnforce(req, "json"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func Test_CheckHeadersEnforce_InvalidMediaType_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.v1+zip")

	err := checkHeadersEnforce(req, "json")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "unsupported media type: zip" {
		t.Errorf("expected 'unsupported media type: zip', got %s", err.Error())
	}
}

func Test_CheckHeadersEnforce_InvalidUnParsableVersion_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/vnd.swift.registry.va+json")

	err := checkHeadersEnforce(req, "json")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "invalid API version: a" {
		t.Errorf("expected 'invalid API version: a', got %s", err.Error())
	}
}

func Test_NewHeaderError_EmptyMessage_ReturnsEmptyErrorMessage(t *testing.T) {
	err := NewHeaderError("")
	if err.Error() != "" {
		t.Errorf("expected empty error message, got %s", err.Error())
	}
}

func Test_CheckHeaders_MultipleAcceptHeaders_ValidHeader_ReturnsNil(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Add("Accept", "application/vnd.swift.registry.v1+json")
	req.Header.Add("Accept", "application/vnd.swift.registry.v1+swift")

	if err := checkHeaders(req); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func Test_CheckHeadersEnforce_MissingAcceptHeader_ReturnsError(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	err := checkHeadersEnforce(req, "json")
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "missing Accept header" {
		t.Errorf("expected 'missing Accept header', got %s", err.Error())
	}
}

func Test_ListElements_RepoError_ReturnsEmptyList(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Controller{repo: &MockRepo{shouldError: true}}
	elements, _ := listElements(w, c, "testScope", "testPackage")

	if len(elements) != 0 {
		t.Errorf("expected empty list, got %d elements", len(elements))
	}
}

func Test_ListElements_RepoError_WritesError(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Controller{repo: &MockRepo{shouldError: true}}
	_, err := listElements(w, c, "testScope", "testPackage")

	if err == nil || w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var response responses.Error
	json.NewDecoder(w.Body).Decode(&response)
	if response.Detail != "error listing package testScope.testPackage" {
		t.Errorf("expected 'error listing package testScope.testPackage', got %s", response.Detail)
	}
}

func Test_ListElements_NilElements_ReturnsEmptyList(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Controller{repo: &MockListElementsRepo{}}
	elements, _ := listElements(w, c, "testScope", "testPackage")

	if len(elements) != 0 {
		t.Errorf("expected empty list, got %d elements", len(elements))
	}
}

func Test_ListElements_EmptyList_ReturnsEmptyList(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Controller{repo: &MockRepo{}}
	elements, _ := listElements(w, c, "testScope", "testPackage")

	if len(elements) != 0 {
		t.Errorf("expected empty list, got %d elements", len(elements))
	}
}

func Test_ListElements_MultipleElements_ReturnsElements(t *testing.T) {
	w := httptest.NewRecorder()
	c := &Controller{repo: &MockListElementsRepo{
		elements: []models.ListElement{
			{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"},
			{Scope: "testScope", PackageName: "testPackage", Version: "2.0.0"},
		},
	}}
	elements, _ := listElements(w, c, "testScope", "testPackage")

	if len(elements) != 2 {
		t.Errorf("expected empty list, got %d elements", len(elements))
	}

	if elements[1].Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", elements[0].Version)
	}
	if elements[0].Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", elements[1].Version)
	}
}

func Test_AddLinkHeaders_EmptyElements_DoesNotSetHeader(t *testing.T) {
	header := http.Header{}
	c := &Controller{}
	addLinkHeaders([]models.ListElement{}, "", c, header)

	if link := header.Get("Link"); link != "" {
		t.Errorf("expected empty Link header, got %s", link)
	}
}

func Test_AddLinkHeaders_SingleElement_SetsLatestHeaderOnly(t *testing.T) {
	header := http.Header{}
	serverConfig := config.ServerConfig{Hostname: "localhost", Port: 8080}
	c := &Controller{config: serverConfig}
	elements := []models.ListElement{
		{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"},
	}
	addLinkHeaders(elements, "1.0.0", c, header)

	expectedLink := "<http://localhost:8080/testScope/testPackage/1.0.0>; rel=\"latest-version\""
	if link := header.Get("Link"); link != expectedLink {
		t.Errorf("expected Link header %q, got %q", expectedLink, link)
	}
}

func Test_AddLinkHeaders_MultipleElements_SetsCorrectHeaders(t *testing.T) {
	header := http.Header{}
	serverConfig := config.ServerConfig{Hostname: "localhost", Port: 8080}
	c := &Controller{config: serverConfig}
	elements := []models.ListElement{
		{Scope: "testScope", PackageName: "testPackage", Version: "2.0.0"}, // Latest
		{Scope: "testScope", PackageName: "testPackage", Version: "1.1.0"}, // Current
		{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"}, // Predecessor
	}
	currentVersion := "1.1.0"
	addLinkHeaders(elements, currentVersion, c, header)

	// Expected order: latest, predecessor, successor
	expectedLink := strings.Join([]string{
		"<http://localhost:8080/testScope/testPackage/2.0.0>; rel=\"latest-version\"",
		"<http://localhost:8080/testScope/testPackage/1.0.0>; rel=\"predecessor-version\"",
		"<http://localhost:8080/testScope/testPackage/2.0.0>; rel=\"successor-version\"",
	}, ", ")

	if link := header.Get("Link"); link != expectedLink {
		t.Errorf("expected Link header %q, got %q", expectedLink, link)
	}
}

func Test_LocationOfElement_ValidElement_ReturnsCorrectLocation(t *testing.T) {
	c := &Controller{config: config.ServerConfig{
		Hostname: "example.com",
		Port:     80,
	}}
	element := models.ListElement{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"}
	location := locationOfElement(c, element)

	expected := "http://example.com/testScope/testPackage/1.0.0"
	if location != expected {
		t.Errorf("expected %s, got %s", expected, location)
	}
}

func Test_LocationOfElement_ValidElementWithTls_ReturnsCorrectLocation(t *testing.T) {
	c := &Controller{config: config.ServerConfig{
		Hostname:   "example.com",
		Port:       443,
		TlsEnabled: true,
	}}
	element := models.ListElement{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"}
	location := locationOfElement(c, element)

	expected := "https://example.com/testScope/testPackage/1.0.0"
	if location != expected {
		t.Errorf("expected %s, got %s", expected, location)
	}
}

func Test_LocationOfElement_ValidElementWithCustomPort_ReturnsCorrectLocation(t *testing.T) {
	c := &Controller{config: config.ServerConfig{
		Hostname: "example.com",
		Port:     8080,
	}}
	element := models.ListElement{Scope: "testScope", PackageName: "testPackage", Version: "1.0.0"}
	location := locationOfElement(c, element)

	expected := "http://example.com:8080/testScope/testPackage/1.0.0"
	if location != expected {
		t.Errorf("expected %s, got %s", expected, location)
	}
}

func Test_WriteError_ValidError_WritesError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError("test error message", w)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	var response responses.Error
	json.NewDecoder(w.Body).Decode(&response)
	if response.Detail != "test error message" {
		t.Errorf("expected 'test error message', got %s", response.Detail)
	}
}

func Test_WriteErrorWithStatusCode_ValidError_WritesError(t *testing.T) {
	w := httptest.NewRecorder()
	writeErrorWithStatusCode("test error message", w, http.StatusConflict)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	var response responses.Error
	json.NewDecoder(w.Body).Decode(&response)
	if response.Detail != "test error message" {
		t.Errorf("expected 'test error message', got %s", response.Detail)
	}
}

func Test_PrintCallInfo_DebugEnabled_LogsRequestInfo(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Custom-Header", "custom-value")

	var logOutput strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))

	printCallInfo("TestMethod", req)

	if !strings.Contains(logOutput.String(), "Header: Authorization=\"Bearer ****\"") {
		t.Errorf("expected log to contain 'Header: Authorization=Bearer ****', got %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), "Header: Custom-Header=custom-value") {
		t.Errorf("expected log to contain 'Header: Custom-Header=custom-value', got %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), "URL url=/test") {
		t.Errorf("expected log to contain 'URL url=/test', got %s", logOutput.String())
	}
	if !strings.Contains(logOutput.String(), "Method method=GET") {
		t.Errorf("expected log to contain 'Method method GET', got %s", logOutput.String())
	}
}

func Test_PrintCallInfo_DebugDisabled_DoesNotLogRequestInfo(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Custom-Header", "custom-value")

	var logOutput strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelInfo})))

	printCallInfo("TestMethod", req)

	if logOutput.String() != "" {
		t.Errorf("expected no log output, got %s", logOutput.String())
	}
}

func Test_PrintCallInfo_BasicAuthHeader_LogsMaskedHeader(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic token")

	var logOutput strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))

	printCallInfo("TestMethod", req)

	if !strings.Contains(logOutput.String(), "Header: Authorization=\"Basic ****\"") {
		t.Errorf("expected log to contain 'Header: Authorization=\"Basic ****\"', got %s", logOutput.String())
	}
}

func Test_PrintCallInfo_OtherAuthHeader_LogsUnmaskedHeader(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})))
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Other token")

	var logOutput strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug})))

	printCallInfo("TestMethod", req)

	if !strings.Contains(logOutput.String(), "Header: Authorization=\"Other token\"") {
		t.Errorf("expected log to contain 'Header: Authorization=\"Other token\"', got %s", logOutput.String())
	}
}

// Mock types and implementations

type MockRepo struct {
	shouldError bool
}

func (m *MockRepo) Exists(element *models.UploadElement) bool {
	return false
}

func (m *MockRepo) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	return nil, nil
}

func (m *MockRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return nil, nil
}

func (m *MockRepo) ExtractManifestFiles(element *models.UploadElement) error {
	return nil
}

func (m *MockRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *MockRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	return time.Time{}, nil
}

func (m *MockRepo) Checksum(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m *MockRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
	return nil, nil
}

func (m *MockRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	return nil, nil
}

func (m *MockRepo) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	return "", nil
}

func (m *MockRepo) Lookup(url string) []string {
	return nil
}

func (m *MockRepo) Remove(element *models.UploadElement) error {
	return nil
}

func (m *MockRepo) List(scope, packageName string) ([]models.ListElement, error) {
	if m.shouldError {
		return nil, fmt.Errorf("simulated error")
	}
	return nil, nil
}

type MockListElementsRepo struct {
	shouldError bool
	elements    []models.ListElement
}

func (m MockListElementsRepo) Exists(element *models.UploadElement) bool {
	return false
}

func (m MockListElementsRepo) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	return nil, nil
}

func (m MockListElementsRepo) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return nil, nil
}

func (m MockListElementsRepo) ExtractManifestFiles(element *models.UploadElement) error {
	return nil
}

func (m MockListElementsRepo) List(scope string, name string) ([]models.ListElement, error) {
	if m.shouldError {
		return nil, fmt.Errorf("simulated error")
	}
	return m.elements, nil
}

func (m MockListElementsRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m MockListElementsRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	return time.Time{}, nil
}

func (m MockListElementsRepo) Checksum(element *models.UploadElement) (string, error) {
	return "", nil
}

func (m MockListElementsRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
	return nil, nil
}

func (m MockListElementsRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	return nil, nil
}

func (m MockListElementsRepo) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	return "", nil
}

func (m MockListElementsRepo) Lookup(url string) []string {
	return nil
}

func (m MockListElementsRepo) Remove(element *models.UploadElement) error {
	return nil
}
