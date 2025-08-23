package models

import (
	"testing"
	"time"
)

func TestInMemoryOperationStore_Store(t *testing.T) {
	store := NewInMemoryOperationStore()
	
	op := &AsyncOperation{
		ID:        "test-123",
		Status:    OperationStatusProcessing,
		CreatedAt: time.Now(),
		Scope:     "example",
		Package:   "test-package",
		Version:   "1.0.0",
	}
	
	// Test storing operation
	err := store.Store(op)
	if err != nil {
		t.Fatalf("Failed to store operation: %v", err)
	}
	
	// Verify it was stored
	retrieved, err := store.Get("test-123")
	if err != nil {
		t.Fatalf("Failed to get operation: %v", err)
	}
	
	if retrieved == nil {
		t.Fatal("Expected to retrieve operation, got nil")
	}
	
	if retrieved.ID != op.ID {
		t.Errorf("Expected ID %s, got %s", op.ID, retrieved.ID)
	}
}

func TestInMemoryOperationStore_Get(t *testing.T) {
	store := NewInMemoryOperationStore()
	
	// Test getting non-existent operation
	op, err := store.Get("non-existent")
	if err != nil {
		t.Fatalf("Get should not return error for non-existent operation: %v", err)
	}
	
	if op != nil {
		t.Error("Expected nil for non-existent operation")
	}
	
	// Store an operation
	testOp := &AsyncOperation{
		ID:        "test-456",
		Status:    OperationStatusCompleted,
		CreatedAt: time.Now(),
	}
	store.Store(testOp)
	
	// Test getting existing operation
	retrieved, err := store.Get("test-456")
	if err != nil {
		t.Fatalf("Failed to get operation: %v", err)
	}
	
	if retrieved == nil {
		t.Fatal("Expected to retrieve operation, got nil")
	}
	
	if retrieved.Status != OperationStatusCompleted {
		t.Errorf("Expected status %s, got %s", OperationStatusCompleted, retrieved.Status)
	}
}

func TestInMemoryOperationStore_Update(t *testing.T) {
	store := NewInMemoryOperationStore()
	
	// Store initial operation
	op := &AsyncOperation{
		ID:        "test-789",
		Status:    OperationStatusProcessing,
		CreatedAt: time.Now(),
	}
	store.Store(op)
	
	// Update the operation
	now := time.Now()
	op.Status = OperationStatusCompleted
	op.CompletedAt = &now
	op.Result = &OperationResult{
		Location: "/example/test-package/1.0.0",
		Message:  "Success",
	}
	
	err := store.Update(op)
	if err != nil {
		t.Fatalf("Failed to update operation: %v", err)
	}
	
	// Verify update
	retrieved, _ := store.Get("test-789")
	if retrieved.Status != OperationStatusCompleted {
		t.Errorf("Expected status %s, got %s", OperationStatusCompleted, retrieved.Status)
	}
	
	if retrieved.Result == nil {
		t.Fatal("Expected result to be set")
	}
	
	if retrieved.Result.Location != "/example/test-package/1.0.0" {
		t.Errorf("Expected location %s, got %s", "/example/test-package/1.0.0", retrieved.Result.Location)
	}
}

func TestInMemoryOperationStore_DeleteExpired(t *testing.T) {
	store := NewInMemoryOperationStore()
	
	// Create operations with different ages
	oldOp := &AsyncOperation{
		ID:        "old-op",
		Status:    OperationStatusCompleted,
		CreatedAt: time.Now().Add(-25 * time.Hour), // 25 hours old
	}
	
	recentOp := &AsyncOperation{
		ID:        "recent-op",
		Status:    OperationStatusCompleted,
		CreatedAt: time.Now().Add(-1 * time.Hour), // 1 hour old
	}
	
	store.Store(oldOp)
	store.Store(recentOp)
	
	// Delete operations older than 24 hours
	err := store.DeleteExpired(24 * time.Hour)
	if err != nil {
		t.Fatalf("Failed to delete expired operations: %v", err)
	}
	
	// Old operation should be deleted
	retrieved, _ := store.Get("old-op")
	if retrieved != nil {
		t.Error("Expected old operation to be deleted")
	}
	
	// Recent operation should still exist
	retrieved, _ = store.Get("recent-op")
	if retrieved == nil {
		t.Error("Expected recent operation to still exist")
	}
}

func TestOperationStatus_Values(t *testing.T) {
	// Test that status constants have expected values
	if OperationStatusProcessing != "processing" {
		t.Errorf("Expected OperationStatusProcessing to be 'processing', got %s", OperationStatusProcessing)
	}
	
	if OperationStatusCompleted != "completed" {
		t.Errorf("Expected OperationStatusCompleted to be 'completed', got %s", OperationStatusCompleted)
	}
	
	if OperationStatusFailed != "failed" {
		t.Errorf("Expected OperationStatusFailed to be 'failed', got %s", OperationStatusFailed)
	}
}