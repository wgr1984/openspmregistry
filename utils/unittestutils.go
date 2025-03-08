package utils

import (
	"archive/zip"
	"errors"
)

// Unit test helper functions
// Use these functions in your unit tests to avoid code duplication
// !!! Tests only, do not use in production code !!!

type ErrorReadCloser struct{}

func (e *ErrorReadCloser) Read(p []byte) (n int, err error) {
	return 0, errors.New("simulated read error")
}

func (e *ErrorReadCloser) Close() error {
	return errors.New("simulated read error")
}

type ErrorZipReadCloser struct {
	zip.ReadCloser
}

func (e *ErrorZipReadCloser) Close() error {
	return errors.New("simulated zip close error")
}

type SuccessReadCloser struct{}

func (s *SuccessReadCloser) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (s *SuccessReadCloser) Close() error {
	return nil
}
