package repo

import (
	"OpenSPMRegistry/models"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func Test_Exists_FileDoesNotExist_ReturnsFalse(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	exists := fileRepo.Exists(element)
	if exists {
		t.Errorf("expected false, got true")
	}
}

func Test_Exists_FileExists_ReturnsTrue(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	exists := fileRepo.Exists(element)
	if !exists {
		t.Errorf("expected true, got false")
	}

	err = os.Remove(path)
	if err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}
}

func Test_GetWriter_ValidElement_ReturnsWriter(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	writer, err := fileRepo.GetWriter(element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer == nil {
		t.Errorf("expected writer, got nil")
	}
	writer.Close()

	// delete the file
	fileRepo.Remove(element)
}

func Test_GetWriter_InvalidPath_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/invalid_path")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	_, err := fileRepo.GetWriter(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_Checksum_FileDoesNotExist_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/not/existing/path")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	_, err := fileRepo.Checksum(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_Checksum_FileExists_ReturnsChecksum(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|os.O_SYNC, 0600)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.WriteString("test data")
	file.Close()

	checksum, err := fileRepo.Checksum(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if checksum == "" {
		t.Errorf("expected checksum, got empty string")
	}

	// check if checksum is correct
	if checksum != "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9" {
		t.Errorf("expected checksum, got %s", checksum)
	}

	// delete the file
	err = os.Remove(path)
	if err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}
}

func Test_Checksum_FileReadError_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	// Simulate read error by removing the file
	os.Remove(path)

	_, err = fileRepo.Checksum(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_Remove_FileExists_RemovesFile(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	err = fileRepo.Remove(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be removed, but it still exists")
	}
}

func Test_Remove_FileDoesNotExist_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/does/not/exist")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	err := fileRepo.Remove(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}
