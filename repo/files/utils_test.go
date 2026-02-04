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
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer func() { _ = file.Close() }()

	zipWriter := zip.NewWriter(file)
	defer func() { _ = zipWriter.Close() }()

	fileWriter, err := zipWriter.Create("Package.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := fileWriter.Write([]byte("swift package content")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

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
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.WriteString("invalid zip content"); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

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
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer func() { _ = file.Close() }()

	zipWriter := zip.NewWriter(file)
	defer func() { _ = zipWriter.Close() }()

	fileWriter, err := zipWriter.Create("NotPackage.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := fileWriter.Write([]byte("not a package swift content")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

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
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer func() { _ = file.Close() }()

	zipWriter := zip.NewWriter(file)
	defer func() { _ = zipWriter.Close() }()

	fileWriter, err := zipWriter.Create("testScope.testName/Package.swift")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := fileWriter.Write([]byte("swift package content")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

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

func Test_ExtractPackageSwiftFiles_AllowsSingleTopLevelDirectory(t *testing.T) {
	defer teardown(t)

	element := models.NewUploadElement(
		"ext",
		"RxSwift",
		"6.9.0",
		mimetypes.ApplicationZip,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	defer func() { _ = file.Close() }()

	zipWriter := zip.NewWriter(file)

	rootManifest, err := zipWriter.Create("ext.RxSwift/Package.swift")
	if err != nil {
		t.Fatalf("failed to create root entry: %v", err)
	}
	if _, err := rootManifest.Write([]byte("root-package")); err != nil {
		t.Fatalf("failed to write root manifest: %v", err)
	}

	nestedManifest, err := zipWriter.Create("ext.RxSwift/Tests/Package.swift")
	if err != nil {
		t.Fatalf("failed to create nested entry: %v", err)
	}
	if _, err := nestedManifest.Write([]byte("nested-package")); err != nil {
		t.Fatalf("failed to write nested manifest: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	var extractedNames []string
	var extractedContents []string
	err = ExtractPackageSwiftFiles(element, path, func(name string, r io.ReadCloser) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		extractedNames = append(extractedNames, name)
		extractedContents = append(extractedContents, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(extractedNames) != 1 {
		t.Fatalf("expected 1 manifest extracted, got %d", len(extractedNames))
	}

	if extractedNames[0] != "Package.swift" {
		t.Errorf("expected root Package.swift, got %s", extractedNames[0])
	}

	if extractedContents[0] != "root-package" {
		t.Errorf("expected root manifest content, got %s", extractedContents[0])
	}
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
