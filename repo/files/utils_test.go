package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func Test_ExtractPackageSwiftFiles_ValidZip_ExtractsFiles(t *testing.T) {
	defer teardown(t)

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationZip,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	fileWriter, err := zipWriter.Create("Package.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	fileWriter.Write([]byte("swift package content"))

	err = zipWriter.Close()
	if err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	err = ExtractPackageSwiftFiles(element, path, func(name string, r io.ReadCloser) error {
		if name != "Package.swift" {
			t.Errorf("unexpected file name: %s", name)
		}
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_ExtractPackageSwiftFiles_InvalidZip_ReturnsError(t *testing.T) {
	defer teardown(t)

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationZip,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	file.WriteString("invalid zip content")

	err = ExtractPackageSwiftFiles(element, path, func(name string, r io.ReadCloser) error {
		return nil
	})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ExtractPackageSwiftFiles_NoPackageSwiftFiles_NoError(t *testing.T) {
	defer teardown(t)

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationZip,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	fileWriter, err := zipWriter.Create("NotPackage.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	fileWriter.Write([]byte("not a package swift content"))

	err = zipWriter.Close()
	if err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	err = ExtractPackageSwiftFiles(element, path, func(name string, r io.ReadCloser) error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_ExtractPackageSwiftFiles_ReadError_ReturnsError(t *testing.T) {
	defer teardown(t)

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationZip,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	fileWriter, err := zipWriter.Create("Package.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	fileWriter.Write([]byte("swift package content"))

	err = zipWriter.Close()
	if err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	err = ExtractPackageSwiftFiles(element, path, func(name string, r io.ReadCloser) error {
		return errors.New("simulated read error")
	})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ExtractPackageSwiftFiles_CloseError_OnFileInsideZipFile_ReturnsError(t *testing.T) {
	// to be implemented
}

func Test_EnsureReaderClosed_CloseFail_ReturnsReaderError(t *testing.T) {
	closer := &utils.ErrorReadCloser{}

	err := ensureReaderClosed(closer)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	if err.Error() != "simulated read error" {
		t.Errorf("expected 'simulated read error', got %s", err.Error())
	}
}

func Test_EnsureReaderClosed_CloseFail_ReturnsNil(t *testing.T) {
	readerCloser := &utils.SuccessReadCloser{}

	err := ensureReaderClosed(readerCloser)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
