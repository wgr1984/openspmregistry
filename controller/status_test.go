package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStatusAction_AsyncDisabled(t *testing.T) {
	// Create controller with async disabled
	cfg := config.ServerConfig{
		Async: config.AsyncConfig{
			Enabled: false,
		},
	}
	
	controller := NewController(cfg, nil)
	
	// Create request
	req, err := http.NewRequest("GET", "/example/test-package/1.0.0/status/test-123", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.SetPathValue("scope", "example")
	req.SetPathValue("package", "test-package")
	req.SetPathValue("version", "1.0.0")
	req.SetPathValue("operation_id", "test-123")
	
	// Record response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.StatusAction)
	handler.ServeHTTP(rr, req)
	
	// Check status code
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, status)
	}
	
	// Check error message
	var errorResp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err == nil {
		if problem, ok := errorResp["detail"].(string); ok {
			if problem != "Async mode is not enabled" {
				t.Errorf("Expected error 'Async mode is not enabled', got '%s'", problem)
			}
		}
	}
}

func TestStatusAction_OperationNotFound(t *testing.T) {
	// Create controller with async enabled
	cfg := config.ServerConfig{
		Async: config.AsyncConfig{
			Enabled: true,
			Workers: 2,
		},
	}
	
	controller := NewController(cfg, nil)
	
	// Create request for non-existent operation
	req, err := http.NewRequest("GET", "/example/test-package/1.0.0/status/non-existent", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.SetPathValue("scope", "example")
	req.SetPathValue("package", "test-package")
	req.SetPathValue("version", "1.0.0")
	req.SetPathValue("operation_id", "non-existent")
	
	// Record response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.StatusAction)
	handler.ServeHTTP(rr, req)
	
	// Check status code
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, status)
	}
}

func TestStatusAction_Success(t *testing.T) {
	// Create controller with async enabled
	cfg := config.ServerConfig{
		Async: config.AsyncConfig{
			Enabled: true,
			Workers: 2,
		},
	}
	
	controller := NewController(cfg, nil)
	
	// Store a test operation
	now := time.Now()
	testOp := &models.AsyncOperation{
		ID:        "test-operation-123",
		Status:    models.OperationStatusCompleted,
		CreatedAt: now,
		CompletedAt: &now,
		Result: &models.OperationResult{
			Location: "/example/test-package/1.0.0",
			Message:  "Package published successfully",
		},
		Scope:   "example",
		Package: "test-package",
		Version: "1.0.0",
	}
	
	controller.operationStore.Store(testOp)
	
	// Create request
	req, err := http.NewRequest("GET", "/example/test-package/1.0.0/status/test-operation-123", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.SetPathValue("scope", "example")
	req.SetPathValue("package", "test-package")
	req.SetPathValue("version", "1.0.0")
	req.SetPathValue("operation_id", "test-operation-123")
	
	// Record response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.StatusAction)
	handler.ServeHTTP(rr, req)
	
	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
	
	// Check headers
	if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
	
	if contentVersion := rr.Header().Get("Content-Version"); contentVersion != "1" {
		t.Errorf("Expected Content-Version '1', got '%s'", contentVersion)
	}
	
	// Check response body
	var response models.AsyncOperation
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.ID != testOp.ID {
		t.Errorf("Expected ID %s, got %s", testOp.ID, response.ID)
	}
	
	if response.Status != models.OperationStatusCompleted {
		t.Errorf("Expected status %s, got %s", models.OperationStatusCompleted, response.Status)
	}
	
	if response.Result == nil {
		t.Fatal("Expected result to be present")
	}
	
	if response.Result.Location != testOp.Result.Location {
		t.Errorf("Expected location %s, got %s", testOp.Result.Location, response.Result.Location)
	}
}

func TestStatusAction_ProcessingOperation(t *testing.T) {
	// Create controller with async enabled
	cfg := config.ServerConfig{
		Async: config.AsyncConfig{
			Enabled: true,
			Workers: 2,
		},
	}
	
	controller := NewController(cfg, nil)
	
	// Store a processing operation
	testOp := &models.AsyncOperation{
		ID:        "processing-op-456",
		Status:    models.OperationStatusProcessing,
		CreatedAt: time.Now(),
		Scope:     "example",
		Package:   "test-package",
		Version:   "1.0.0",
	}
	
	controller.operationStore.Store(testOp)
	
	// Create request
	req, err := http.NewRequest("GET", "/example/test-package/1.0.0/status/processing-op-456", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.SetPathValue("scope", "example")
	req.SetPathValue("package", "test-package")
	req.SetPathValue("version", "1.0.0")
	req.SetPathValue("operation_id", "processing-op-456")
	
	// Record response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.StatusAction)
	handler.ServeHTTP(rr, req)
	
	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
	
	// Check response body
	var response models.AsyncOperation
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Status != models.OperationStatusProcessing {
		t.Errorf("Expected status %s, got %s", models.OperationStatusProcessing, response.Status)
	}
	
	if response.CompletedAt != nil {
		t.Error("Expected CompletedAt to be nil for processing operation")
	}
}

func TestStatusAction_FailedOperation(t *testing.T) {
	// Create controller with async enabled
	cfg := config.ServerConfig{
		Async: config.AsyncConfig{
			Enabled: true,
			Workers: 2,
		},
	}
	
	controller := NewController(cfg, nil)
	
	// Store a failed operation
	now := time.Now()
	testOp := &models.AsyncOperation{
		ID:          "failed-op-789",
		Status:      models.OperationStatusFailed,
		CreatedAt:   now,
		CompletedAt: &now,
		Error: &models.OperationError{
			Code:    "validation_failed",
			Message: "Invalid package format",
		},
		Scope:   "example",
		Package: "test-package",
		Version: "1.0.0",
	}
	
	controller.operationStore.Store(testOp)
	
	// Create request
	req, err := http.NewRequest("GET", "/example/test-package/1.0.0/status/failed-op-789", nil)
	if err != nil {
		t.Fatal(err)
	}
	
	req.SetPathValue("scope", "example")
	req.SetPathValue("package", "test-package")
	req.SetPathValue("version", "1.0.0")
	req.SetPathValue("operation_id", "failed-op-789")
	
	// Record response
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.StatusAction)
	handler.ServeHTTP(rr, req)
	
	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, status)
	}
	
	// Check response body
	var response models.AsyncOperation
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	
	if response.Status != models.OperationStatusFailed {
		t.Errorf("Expected status %s, got %s", models.OperationStatusFailed, response.Status)
	}
	
	if response.Error == nil {
		t.Fatal("Expected error to be present")
	}
	
	if response.Error.Code != "validation_failed" {
		t.Errorf("Expected error code 'validation_failed', got '%s'", response.Error.Code)
	}
}