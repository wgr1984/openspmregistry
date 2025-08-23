package models

import (
	"sync"
	"time"
)

// OperationStatus represents the status of an async operation
type OperationStatus string

const (
	// OperationStatusProcessing indicates the operation is still in progress
	OperationStatusProcessing OperationStatus = "processing"
	// OperationStatusCompleted indicates the operation completed successfully
	OperationStatusCompleted OperationStatus = "completed"
	// OperationStatusFailed indicates the operation failed
	OperationStatusFailed OperationStatus = "failed"
)

// AsyncOperation represents an asynchronous package operation
type AsyncOperation struct {
	// ID is the unique identifier for this operation
	ID string `json:"id"`
	
	// Status is the current status of the operation
	Status OperationStatus `json:"status"`
	
	// CreatedAt is when the operation was created
	CreatedAt time.Time `json:"created_at"`
	
	// CompletedAt is when the operation completed (if applicable)
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	
	// Result contains success information (if completed successfully)
	Result *OperationResult `json:"result,omitempty"`
	
	// Error contains error information (if failed)
	Error *OperationError `json:"error,omitempty"`
	
	// Package information
	Scope   string `json:"-"`
	Package string `json:"-"`
	Version string `json:"-"`
}

// OperationResult contains information about a successful operation
type OperationResult struct {
	// Location is the URL of the published package
	Location string `json:"location"`
	
	// Message is an optional success message
	Message string `json:"message,omitempty"`
}

// OperationError contains information about a failed operation
type OperationError struct {
	// Code is a machine-readable error code
	Code string `json:"code"`
	
	// Message is a human-readable error message
	Message string `json:"message"`
}

// AsyncOperationStore provides storage for async operations
type AsyncOperationStore interface {
	// Store saves an async operation
	Store(op *AsyncOperation) error
	
	// Get retrieves an async operation by ID
	Get(id string) (*AsyncOperation, error)
	
	// Update updates an existing async operation
	Update(op *AsyncOperation) error
	
	// DeleteExpired removes operations older than the given duration
	DeleteExpired(ttl time.Duration) error
}

// InMemoryOperationStore is a simple in-memory implementation of AsyncOperationStore
type InMemoryOperationStore struct {
	mu         sync.RWMutex
	operations map[string]*AsyncOperation
}

// NewInMemoryOperationStore creates a new in-memory operation store
func NewInMemoryOperationStore() *InMemoryOperationStore {
	return &InMemoryOperationStore{
		operations: make(map[string]*AsyncOperation),
	}
}

// Store saves an async operation
func (s *InMemoryOperationStore) Store(op *AsyncOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.operations[op.ID] = op
	return nil
}

// Get retrieves an async operation by ID
func (s *InMemoryOperationStore) Get(id string) (*AsyncOperation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	op, exists := s.operations[id]
	if !exists {
		return nil, nil
	}
	return op, nil
}

// Update updates an existing async operation
func (s *InMemoryOperationStore) Update(op *AsyncOperation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.operations[op.ID] = op
	return nil
}

// DeleteExpired removes operations older than the given duration
func (s *InMemoryOperationStore) DeleteExpired(ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	cutoff := time.Now().Add(-ttl)
	for id, op := range s.operations {
		if op.CreatedAt.Before(cutoff) {
			delete(s.operations, id)
		}
	}
	return nil
}