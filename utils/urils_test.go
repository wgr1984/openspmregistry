package utils

import (
	"OpenSPMRegistry/config"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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
}

func Test_RandomString_InvalidLength_ReturnsError(t *testing.T) {
	_, err := RandomString(-1)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_BaseUrl_ValidConfig_ReturnsUrl(t *testing.T) {
	config := config.ServerConfig{Hostname: "localhost", Port: 8080}
	expected := "https://localhost:8080"
	result := BaseUrl(config)
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
