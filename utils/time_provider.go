package utils

import "time"

// TimeProvider defines the interface for getting the current time
type TimeProvider interface {
	Now() time.Time
}

// RealTimeProvider provides actual system time
type RealTimeProvider struct{}

func (p *RealTimeProvider) Now() time.Time {
	return time.Now()
}

// NewRealTimeProvider creates a new RealTimeProvider
func NewRealTimeProvider() *RealTimeProvider {
	return &RealTimeProvider{}
}

// MockTimeProvider provides a fixed time for testing
type MockTimeProvider struct {
	fixedTime time.Time
}

func (p *MockTimeProvider) Now() time.Time {
	return p.fixedTime
}

// NewMockTimeProvider creates a new MockTimeProvider with a fixed time
func NewMockTimeProvider(fixedTime time.Time) *MockTimeProvider {
	return &MockTimeProvider{fixedTime: fixedTime}
}
