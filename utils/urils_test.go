package utils

import (
	"OpenSPMRegistry/config"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
)

func Test_CopyStruct_ValidStruct_ReturnsCopy(t *testing.T) {
	type TestStruct struct {
		Field1 string
		Field2 int
	}
	original := &TestStruct{Field1: "test", Field2: 123}
	copy := CopyStruct(original)
	if copy.Field1 != original.Field1 || copy.Field2 != original.Field2 {
		t.Errorf("expected %v, got %v", original, copy)
	}
}

func Test_StripExtension_HasExtension_ReturnsStripped(t *testing.T) {
	result := StripExtension("filename.txt", ".txt")
	expected := "filename"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_StripExtension_NoExtension_ReturnsOriginal(t *testing.T) {
	result := StripExtension("filename", ".txt")
	expected := "filename"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_RandomString_ValidLength_ReturnsString(t *testing.T) {
	result, err := RandomString(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 16 { // base64 encoding increases length
		t.Errorf("expected length 16, got %d", len(result))
	}
	result2, err := RandomString(10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == result2 {
		t.Errorf("expected different strings, got same")
	}
}

func Test_RandomString_InvalidLength_ReturnsError(t *testing.T) {
	_, err := RandomString(-1)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_RandomString_GenerateError_ReturnsString(t *testing.T) {
	fakeErr := errors.New("fake error")
	_, err := randomStringFromGenerator(10, iotest.ErrReader(fakeErr))
	if err == nil || !errors.Is(err, fakeErr) {
		t.Errorf("expected error, got nil")
	}
}

func Test_RandomString_ZeroLength_ReturnsEmptyString(t *testing.T) {
	result, err := RandomString(0)
	if err != nil {
		t.Errorf("expected no error for zero length, got %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string for zero length, got %s", result)
	}
}

func Test_RandomString_ReadError_ReturnsError(t *testing.T) {
	// Create a reader that always returns an error
	errReader := &ErrorReadCloser{}
	_, err := randomStringFromGenerator(10, errReader)
	if err == nil {
		t.Error("expected error from reader, got nil")
	}
}

func Test_BaseUrl_ValidConfig_ReturnsUrl(t *testing.T) {
	c := config.ServerConfig{Hostname: "localhost", Port: 8080}
	expected := "http://localhost:8080"
	result := BaseUrl(c)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_BaseUrl_StandardPort_ReturnsUrl(t *testing.T) {
	c := config.ServerConfig{Hostname: "localhost", Port: 80}
	expected := "http://localhost"
	result := BaseUrl(c)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_BaseUrl_TlsEnabled_ReturnsUrl(t *testing.T) {
	config := config.ServerConfig{Hostname: "localhost", Port: 8080, TlsEnabled: true}
	expected := "https://localhost:8080"
	result := BaseUrl(config)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_BaseUrl_TlsEnabled_StandardPort_ReturnsUrl(t *testing.T) {
	c := config.ServerConfig{Hostname: "localhost", Port: 443, TlsEnabled: true}
	expected := "https://localhost"
	result := BaseUrl(c)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_WriteAuthorizationHeaderError_ValidError_WritesError(t *testing.T) {
	w := httptest.NewRecorder()
	err := fmt.Errorf("test error")
	WriteAuthorizationHeaderError(w, err)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	expectedBody := "Authentication failed: test error\n"
	if w.Body.String() != expectedBody {
		t.Errorf("expected body %s, got %s", expectedBody, w.Body.String())
	}
}
