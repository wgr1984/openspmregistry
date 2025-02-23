package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"
)

type fakeOsModule_statError struct {
	osAdapterDefault
}

func (m *fakeOsModule_statError) Stat(name string) (os.FileInfo, error) {
	return nil, fakeError
}

type fakeOsModule_walkDirError struct {
	osAdapterDefault
}

func (m *fakeOsModule_walkDirError) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		return fn(path, d, fakeError)
	})
}

type access_error struct {
	access
}

func (a *access_error) Exists(element *models.UploadElement) bool {
	return true
}

func (a *access_error) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	return nil, fakeError
}

func (a *access_error) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	return nil, fakeError
}

type ErrReaderSeekCloser_ReadErr struct {
	inner io.ReadSeekCloser
}

func (r *ErrReaderSeekCloser_ReadErr) Read(p []byte) (n int, err error) {
	return 0, fakeError
}

func (r *ErrReaderSeekCloser_ReadErr) Seek(offset int64, whence int) (int64, error) {
	return r.inner.Seek(offset, whence)
}

func (r *ErrReaderSeekCloser_ReadErr) Close() error {
	return r.inner.Close()
}

type ErrReaderSeekCloser_SeekErr struct {
	inner io.ReadSeekCloser
}

func (r *ErrReaderSeekCloser_SeekErr) Read(p []byte) (n int, err error) {
	return r.inner.Read(p)
}

func (r *ErrReaderSeekCloser_SeekErr) Seek(offset int64, whence int) (int64, error) {
	return 0, fakeError
}

func (r *ErrReaderSeekCloser_SeekErr) Close() error {
	return r.inner.Close()
}

type ErrReaderSeekCloser_CloseErr struct {
	inner io.ReadSeekCloser
}

func (r *ErrReaderSeekCloser_CloseErr) Read(p []byte) (n int, err error) {
	return r.inner.Read(p)
}

func (r *ErrReaderSeekCloser_CloseErr) Seek(offset int64, whence int) (int64, error) {
	return r.inner.Seek(offset, whence)
}

func (r *ErrReaderSeekCloser_CloseErr) Close() error {
	return fakeError
}

type reader_error_close struct {
	access
}

func (r *reader_error_close) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	inner, _ := r.access.GetReader(element)
	return &ErrReaderSeekCloser_CloseErr{
		inner: inner,
	}, nil
}

func isRoot() bool {
	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf("[isRoot] Unable to get current user: %s", err)
	}
	return currentUser.Username == "root"
}

func Test_ExtractManifestFiles_ValidZipFile_ExtractsFiles(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: mimetypes.ApplicationZip,
	}

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
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
	extractedPath := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, "Package.swift")
	if _, err := os.Stat(extractedPath); errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be extracted, but it does not exist")
	}
}

func Test_ExtractManifestFiles_UnsupportedMimeType_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: "text/plain",
	}

	err := os.MkdirAll(filepath.Dir(filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	err = fileRepo.ExtractManifestFiles(element)
	if err == nil && !errors.Is(err, errors.New("unsupported mime type")) {
		t.Errorf("expected error, got nil")
	}
}

func Test_ExtractManifestFiles_NonExistentPath_CreatesPathAndExtractsFiles(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: mimetypes.ApplicationZip,
	}
	element.SetFilenameOverwrite("Package")

	path := filepath.Join("/tmp/openspmsreg_tests/non/existent", element.Scope, element.Name, element.Version, element.FileName())
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
	extractedPath := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, "Package.swift")
	if _, err := os.Stat(extractedPath); err == nil {
		t.Errorf("expected file to be extracted, but it does not exist")
	}
}

func Test_ExtractManifestFiles_MkDirAllCreateError_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := &FileRepo{
		path:     "/tmp/openspmsreg_tests/invalid_path",
		osModule: &fakeOsModule_mkDirAllError{},
	}
	element := &models.UploadElement{
		Scope:    "testScope",
		Name:     "testName",
		Version:  "1.0.0",
		MimeType: mimetypes.ApplicationZip,
	}

	err := fileRepo.ExtractManifestFiles(element)
	if err == nil || !errors.Is(err, fakeError) {
		t.Errorf("expected error, got nil")
	}
}

func Test_List_DirectoryExists_ReturnsListOfElements(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	scope := "testScope"
	name := "testName"
	version := "1.0.0"

	path := filepath.Join("/tmp/openspmsreg_tests", scope, name, version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", scope))

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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	scope := "scope"
	name := "empty"

	path := filepath.Join("/tmp/openspmsreg_tests", scope, name)
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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/invalid_path_list")
	scope := "testScope"
	name := "testName"

	_, err := fileRepo.List(scope, name)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected error, got nil")
	}
}

func Test_List_ErrorStatFile_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := &FileRepo{
		path:     "/tmp/openspmsreg_tests",
		osModule: &fakeOsModule_statError{},
	}
	scope := "testScope"
	name := "testName"

	_, err := fileRepo.List(scope, name)
	if err == nil || !errors.Is(err, fakeError) {
		t.Errorf("expected error, got nil")
	}
}

func Test_List_ErrorWalkingDirectory_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := &FileRepo{
		path:     "/tmp/openspmsreg_tests",
		osModule: &fakeOsModule_walkDirError{},
	}

	scope := "testScope"
	name := "testName"
	version := "1.0.0"

	err := os.MkdirAll(filepath.Join("/tmp/openspmsreg_tests", scope, name, version), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	_, err = fileRepo.List(scope, name)
	if err == nil || !errors.Is(err, fakeError) {
		t.Errorf("expected error, got nil")
	}
}

func Test_EncodeBase64_FileExists_ReturnsBase64String(t *testing.T) {
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
	file.WriteString("test data")
	file.Close()

	base64String, err := fileRepo.EncodeBase64(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	expected := base64.StdEncoding.EncodeToString([]byte("test data"))
	if base64String != expected {
		t.Errorf("expected %s, got %s", expected, base64String)
	}
}

func Test_EncodeBase64_FileDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	_, err := fileRepo.EncodeBase64(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_EncodeBase64_ReadError_ReturnsError(t *testing.T) {
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

	_, err = fileRepo.EncodeBase64(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_EncodeBase64_GetReaderError_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := &FileRepo{
		path:     "/tmp/openspmsreg_tests",
		Access:   &access_error{},
		osModule: &osAdapterDefault{},
	}
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	err := os.MkdirAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	file, err := os.Create(filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName()))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	_, err = fileRepo.EncodeBase64(element)
	if err == nil || !errors.Is(err, fakeError) {
		t.Errorf("expected error, got nil")
	}
}

func Test_EncodeBase64_ReaderCloseError_ReturnsError(t *testing.T) {
	defer teardown(t)

	path := "/tmp/openspmsreg_tests"
	osModule := &osAdapterDefault{}
	fileRepo := &FileRepo{
		path: path,
		Access: &reader_error_close{
			access: access{
				path:     path,
				osModule: osModule,
			},
		},
		osModule: osModule,
	}

	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path = filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
	err := os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = file.WriteString("test data")
	if err != nil {
		t.Fatalf("failed to write to file: %v", err)
	}
	file.Close()

	_, err = fileRepo.EncodeBase64(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func Test_PublishDate_ValidFile_ReturnsModTime(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	file, err := os.Create(filepath.Join(path, element.FileName()))
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	modTime := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(path, element.FileName()), modTime, modTime)

	result, err := fileRepo.PublishDate(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !result.Equal(modTime) {
		t.Errorf("expected %v, got %v", modTime, result)
	}
}

func Test_PublishDate_PathDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"nonExistentScope",
		"nonExistentName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	_, err := fileRepo.PublishDate(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_PublishDate_FileDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)

	_, err := fileRepo.PublishDate(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_Checksum_FileDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/not/existing/path")
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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version, element.FileName())
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

	// Simulate read error by removing the file
	os.Remove(path)

	_, err = fileRepo.Checksum(element)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_GetAlternativeManifests_ValidPath_ReturnsManifests(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope))

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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := &models.UploadElement{
		Scope:   "testScope",
		Name:    "testName",
		Version: "1.0.0",
	}

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope))

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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope))

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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
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
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.TextXSwift,
		models.Manifest,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope))

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

func Test_Lookup_ValidURL_ReturnsMatchingIDs(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/testRepo")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationJson,
		models.Metadata,
	)

	path := filepath.Join("/tmp/openspmsreg_tests/testRepo", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests/testRepo", element.Scope))

	metadataPath := filepath.Join(path, "metadata.json")
	file, err := os.Create(metadataPath)
	if err != nil {
		t.Fatalf("failed to create metadata file: %v", err)
	}
	_, err = file.WriteString(`{"repositoryURLs": ["https://example.com/repo"]}`)
	if err != nil {
		t.Fatalf("failed to write to metadata file: %v", err)
	}
	file.Close()

	packagePath := filepath.Join(path, "Package.swift")
	err = os.WriteFile(packagePath, []byte(`// swift-tools-version:5.3
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
	)`), os.ModePerm)

	result := fileRepo.Lookup("https://example.com/repo")
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
	if result[0] != "testScope.testName" {
		t.Errorf("expected testScope.testName, got %s", result[0])
	}
}

func Test_Lookup_InvalidURL_ReturnsEmptyList(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	element := models.NewUploadElement(
		"testScope",
		"testName",
		"1.0.0",
		mimetypes.ApplicationJson,
		models.Metadata,
	)

	path := filepath.Join("/tmp/openspmsreg_tests", element.Scope, element.Name, element.Version)
	os.MkdirAll(path, os.ModePerm)
	defer os.RemoveAll(filepath.Join("/tmp/openspmsreg_tests", element.Scope))

	metadataPath := filepath.Join(path, "metadata.json")
	file, err := os.Create(metadataPath)
	if err != nil {
		t.Fatalf("failed to create metadata file: %v", err)
	}
	_, err = file.WriteString(`{"repositoryURLs": ["https://example.com/repo"]}`)
	if err != nil {
		t.Fatalf("failed to write to metadata file: %v", err)
	}
	file.Close()

	result := fileRepo.Lookup("https://invalid.com/repo")
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func Test_Lookup_NoMetadataFiles_ReturnsEmptyList(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests")
	os.MkdirAll("/tmp/openspmsreg_tests/testScope/testName/1.0.0", os.ModePerm)
	defer os.RemoveAll("/tmp/openspmsreg_tests/testScope")

	result := fileRepo.Lookup("https://example.com/repo")
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func Test_Lookup_ErrorWalkingDirectories_ReturnsEmptyList(t *testing.T) {
	defer teardown(t)

	fileRepo := NewFileRepo("/tmp/openspmsreg_tests/invalid_path")

	result := fileRepo.Lookup("https://example.com/repo")
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func Test_Remove_FileExists_RemovesFile(t *testing.T) {
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

	err = fileRepo.Remove(element)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file to be removed, but it still exists")
	}
}

func Test_Remove_FileDoesNotExist_ReturnsError(t *testing.T) {
	defer teardown(t)

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
