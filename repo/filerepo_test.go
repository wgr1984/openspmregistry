package repo

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
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

func Test_ExtractManifestFiles_ValidZipFile_ExtractsFiles(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: mimetypes.ApplicationZip,
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	zipWriter := zip.NewWriter(file)

	// sample Package.swift file
	packageSwift, err := zipWriter.Create("Package.swift")
	if err != nil {
		t.Fatalf("failed to create Package.swift file: %v", err)
	}
	_, err = packageSwift.Write([]byte(`
// swift-tools-version:5.3
import PackageDescription

let package = Package(
	name: "SamplePackage",
	platforms: [
		.macOS(.v10_15)
	],
	products: [
		.library(
			name: "SamplePackage",
			targets: ["SamplePackage"]),
	],
	dependencies: [
		// Dependencies declare other packages that this package depends on.
		// .package(url: /* package url */, from: "1.0.0"),
	],
	targets: [
		.target(
			name: "SamplePackage",
			dependencies: []),
		.testTarget(
			name: "SamplePackageTests",
			dependencies: ["SamplePackage"]),
	]
)
`))

	if err != nil {
		t.Fatalf("failed to write to Package.swift file: %v", err)
	}

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	file.Close()

	err = fileRepo.ExtractManifestFiles(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check if files are extracted
	extractedPath := filepath.Join("/tmp", element.Scope, element.Name, element.Version, "Package.swift")
	if _, err := os.Stat(extractedPath); errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be extracted, but it does not exist")
	}
}

func Test_ExtractManifestFiles_UnsupportedMimeType_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: "text/plain",
	}

	err := fileRepo.ExtractManifestFiles(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ExtractManifestFiles_NonExistentPath_CreatesPathAndExtractsFiles(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: mimetypes.ApplicationZip,
	}
	element.SetFilenameOverwrite("Package")

	path := filepath.Join("/tmp/non/existent", element.Scope, element.Name, element.Version, element.FileName())
	os.MkdirAll(filepath.Dir(path), os.ModePerm)

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	file.Close()

	err = fileRepo.ExtractManifestFiles(element)
	if err == nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check if path is created and files are extracted
	extractedPath := filepath.Join("/tmp", element.Scope, element.Name, element.Version, "Package.swift")
	if _, err := os.Stat(extractedPath); errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be extracted, but it does not exist")
	}
}

func Test_List_DirectoryExists_ReturnsListOfElements(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	scope := "testScope"
	name := "testName"
	version := "1.0.0"

	path := filepath.Join("/tmp", scope, name, version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp", scope))

	_, err := os.Create(filepath.Join(path, "dummyFile"))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	elements, err := fileRepo.List(scope, name)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(elements) != 1 {
		t.Errorf("expected 1 element, got %d", len(elements))
	}

	if elements[0].Version != version {
		t.Errorf("expected version %s, got %s", version, elements[0].Version)
	}
}

func Test_List_DirectoryExistButEmpty_ReturnsEmptyList(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	scope := "scope"
	name := "empty"

	path := filepath.Join("/tmp", scope, name)
	os.MkdirAll(path, os.ModePerm)

	elements, err := fileRepo.List(scope, name)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(elements))
	}
}

func Test_List_ErrorReadingDirectory_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/invalid_path")
	scope := "testScope"
	name := "testName"

	_, err := fileRepo.List(scope, name)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
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

func Test_GetAlternativeManifests_ValidPath_ReturnsManifests(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp", element.Scope))

	_, err := os.Create(filepath.Join(path, "Package@swift-7.16.swift"))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	manifests, err := fileRepo.GetAlternativeManifests(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(manifests) != 1 {
		t.Errorf("expected 1 manifest, got %d", len(manifests))
	}

	if manifests[0].FileName() != "Package@swift-7.16.swift" {
		t.Errorf("expected Package1.swift, got %s", manifests[0].FileName())
	}
}

func Test_GetAlternativeManifests_PathDoesNotExist_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "nonExistentScope",
		Name:    "nonExistentName",
		Version: "1.0.0",
	}

	manifests, err := fileRepo.GetAlternativeManifests(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if manifests != nil {
		t.Errorf("expected nil manifests, got %v", manifests)
	}
}

func Test_GetAlternativeManifests_NoAlternativeManifests_ReturnsEmptyList(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp", element.Scope))

	_, err := os.Create(filepath.Join(path, "Package.swift"))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	manifests, err := fileRepo.GetAlternativeManifests(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests, got %d", len(manifests))
	}
}

func Test_GetSwiftToolVersion_ValidManifest_ReturnsVersion(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp", element.Scope))

	file, err := os.Create(filepath.Join(path, element.FileName()))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = file.WriteString("// swift-tools-version:5.3\n")
	if err != nil {
		t.Fatalf("failed to write to file: %v", err)
	}
	file.Close()

	version, err := fileRepo.GetSwiftToolVersion(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if version != "5.3" {
		t.Errorf("expected version 5.3, got %s", version)
	}
}

func Test_GetSwiftToolVersion_FileDoesNotExist_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	_, err := fileRepo.GetSwiftToolVersion(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetSwiftToolVersion_NoSwiftVersion_ReturnsError(t *testing.T) {
	fileRepo := NewFileRepo("/tmp")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp", element.Scope))

	file, err := os.Create(filepath.Join(path, element.FileName()))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = file.WriteString("import PackageDescription\n")
	if err != nil {
		t.Fatalf("failed to write to file: %v", err)
	}
	file.Close()

	_, err = fileRepo.GetSwiftToolVersion(element)
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
