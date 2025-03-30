package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var fakeError = errors.New("fake_error")

type fakeOsModule_mkDirAllError struct {
	osAdapterDefault
}

func (m *fakeOsModule_mkDirAllError) MkdirAll(path string, perm os.FileMode) error {
	return fakeError
}

type fakeOsModule_openError struct {
	osAdapterDefault
}

func (m *fakeOsModule_openError) Open(name string) (*os.File, error) {
	return nil, fakeError
}

func teardown(t *testing.T) {
	err := os.RemoveAll("/tmp/openspmsreg_tests")
	if err != nil {
		t.Fatalf("failed to remove directory: %v", err)
	}
}

func Test_Exists_FileDoesNotExist_ReturnsFalse(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
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

func Test_GetReader_FileExists_ReturnsReader(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	reader, err := fileRepo.GetReader(element)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reader == nil {
		t.Errorf("expected reader, got nil")
	}
	reader.Close()
}

func Test_GetReader_FileDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)
	element.SetFilenameOverwrite("nonExistentFile.txt")

	reader, err := fileRepo.GetReader(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if reader != nil {
		t.Errorf("expected nil reader, got %v", reader)
	}
}

func Test_GetReader_InvalidPath_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/invalid_path")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	).SetFilenameOverwrite("nonExistentFile.txt")

	_, err := fileRepo.GetReader(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetReader_FileReadError_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	// Simulate read error by removing the file
	os.Remove(path)

	_, err = fileRepo.GetReader(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetWriter_ValidElement_ReturnsWriter(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
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
	if isRoot() {
		t.Skip("Skipping testing in Docker environment (root user)")
	}

	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/invalid_path_error")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	// create a directory
	err := os.MkdirAll("/tmp/openspmsreg_tests/invalid_path_error", os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// block permissions to create the file
	err = os.Chmod("/tmp/openspmsreg_tests/invalid_path_error", 0000)
	if err != nil {
		t.Fatalf("failed to change directory permissions: %v", err)
	}

	_, err = fileRepo.GetWriter(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetWriter_GetWriterError_ReturnsError(t *testing.T) {
	defer teardown(t)

	osModule := &osAdapterDefault{}
	fileRepo := &FileRepo{
		path:     "/tmp/openspmsreg_tests/write_error",
		osModule: osModule,
		Access:   &access_error{},
	}
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	// create a directory
	err := os.MkdirAll("/tmp/openspmsreg_tests/write_error", os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err = fileRepo.GetWriter(element)
	if err == nil || !errors.Is(err, fakeError) {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetWriter_MkdirAllError_ReturnsError(t *testing.T) {
	defer teardown(t)

	path := "/tmp/openspmsreg_tests/mkdir_all_error"
	osModule := &fakeOsModule_mkDirAllError{}
	fileRepo := &FileRepo{
		path:     path,
		osModule: osModule,
		Access: &access{
			path:     path,
			osModule: osModule,
		},
	}

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		"testFile",
		"application/zip",
	)

	_, err := fileRepo.GetWriter(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetReader_OpenError_ReturnsError(t *testing.T) {
	defer teardown(t)

	path := "/tmp/openspmsreg_tests/access_read_error"
	osModule := &fakeOsModule_openError{}
	fileRepo := &FileRepo{
		path: path,
		Access: &access{
			path:     path,
			osModule: osModule,
		},
		osModule: osModule,
	}

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		"testFile",
		"application/zip",
	)

	err := os.MkdirAll(filepath.Join(path, element.Scope, element.Name, element.Version), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	err = os.WriteFile(filepath.Join(path, element.Scope, element.Name, element.Version, element.FileName()), []byte("test"), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	_, err = fileRepo.GetReader(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if !errors.Is(err, fakeError) {
		t.Errorf("expected error, got %v", err)
	}
}
