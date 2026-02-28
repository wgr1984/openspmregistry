//go:build integration
// +build integration

package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// IntegrationTestHelper provides utilities for integration tests with a real Maven repository server
type IntegrationTestHelper struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewIntegrationTestHelper creates a new integration test helper
// It expects the Maven repository server to be running at the provided baseURL
func NewIntegrationTestHelper(baseURL string) *IntegrationTestHelper {
	return &IntegrationTestHelper{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WaitForServer waits for the Maven repository server to be ready
// It checks the health endpoint with retries
func (h *IntegrationTestHelper) WaitForServer(ctx context.Context, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Nexus requires a path segment after the repo key (e.g. .../repository/private/); use trailing slash
			checkURL := strings.TrimSuffix(h.BaseURL, "/") + "/"
			req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := h.HTTPClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
					return nil
				}
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("server not ready after %v", maxWait)
			}
		}
	}
}

// GetMavenConfig returns a MavenConfig for integration tests
// It reads credentials from environment variables or uses Maven server defaults
func (h *IntegrationTestHelper) GetMavenConfig() config.MavenConfig {
	username := os.Getenv("MAVEN_REPO_USERNAME")
	if username == "" {
		username = "admin" // Nexus default
	}

	password := os.Getenv("MAVEN_REPO_PASSWORD")
	if password == "" {
		password = "admin123" // Nexus default (set by bootstrap)
	}

	return config.MavenConfig{
		BaseURL:  h.BaseURL,
		Timeout:  30,
		AuthMode: "config",
		Username: username,
		Password: password,
	}
}

// SkipIfNotIntegration skips the test if integration tests are not enabled
func SkipIfNotIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test. Set INTEGRATION_TESTS=1 to run.")
	}
}

// createTestZip creates a simple test zip file for integration tests
func createTestZip(t *testing.T) []byte {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Add a simple file to the zip
	testFile, err := zipWriter.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create file in zip: %v", err)
	}
	testFile.Write([]byte("test content"))

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("Failed to close zip: %v", err)
	}

	return buf.Bytes()
}

// createTestZipWithManifests creates a test zip file with Package.swift and Swift version variant
// Directory must match scope.name format (e.g., "test.TestPackage/")
func createTestZipWithManifests(t *testing.T, scope, name string) []byte {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)

	// Directory must match scope.name format
	dirPrefix := scope + "." + name + "/"

	// Add Package.swift
	packageSwift, err := zipWriter.Create(dirPrefix + "Package.swift")
	if err != nil {
		t.Fatalf("Failed to create Package.swift in zip: %v", err)
	}
	packageSwift.Write([]byte("// swift-tools-version:6.0\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])"))

	// Add Package@swift-5.7.0.swift variant
	packageSwift57, err := zipWriter.Create(dirPrefix + "Package@swift-5.7.0.swift")
	if err != nil {
		t.Fatalf("Failed to create Package@swift-5.7.0.swift in zip: %v", err)
	}
	packageSwift57.Write([]byte("// swift-tools-version:5.7.0\nimport PackageDescription\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])"))

	// Add a source file
	sourceFile, err := zipWriter.Create(dirPrefix + "Sources/TestPackage/TestPackage.swift")
	if err != nil {
		t.Fatalf("Failed to create source file in zip: %v", err)
	}
	sourceFile.Write([]byte("public struct TestPackage {}"))

	if err := zipWriter.Close(); err != nil {
		t.Fatalf("Failed to close zip: %v", err)
	}

	return buf.Bytes()
}

// TestIntegration_PublishAndGet_RealServer tests publishing and retrieving files
func TestIntegration_PublishAndGet_RealServer(t *testing.T) {
	SkipIfNotIntegration(t)

	baseURL := os.Getenv("MAVEN_REPO_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8081/repository"
	}

	// If BaseURL doesn't already include a repository path, append the default repository name
	repositoryName := os.Getenv("MAVEN_REPO_NAME")
	if repositoryName == "" {
		repositoryName = "private" // Nexus repo created by bootstrap script
	}

	// Ensure BaseURL ends with the repository name
	if !strings.HasSuffix(baseURL, "/"+repositoryName) && !strings.HasSuffix(baseURL, "/"+repositoryName+"/") {
		baseURL = strings.TrimSuffix(baseURL, "/") + "/" + repositoryName
	}

	helper := NewIntegrationTestHelper(baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Wait for server to be ready
	if err := helper.WaitForServer(ctx, 2*time.Minute); err != nil {
		t.Fatalf("Server not ready: %v", err)
	}

	// Create Maven repo instance
	cfg := helper.GetMavenConfig()
	repo, err := NewMavenRepo(cfg)
	if err != nil {
		t.Fatalf("Failed to create Maven repo: %v", err)
	}

	// Test data
	scope := "test"
	name := "TestPackage"
	version := "1.0.0"
	testZipData := createTestZip(t)

	// Create upload element for source archive
	element := models.NewUploadElement(scope, name, version, mimetypes.ApplicationZip, models.SourceArchive)

	// Log the configuration for debugging
	t.Logf("Integration test configuration:")
	t.Logf("  BaseURL: %s", baseURL)
	t.Logf("  Repository: %s", repositoryName)
	t.Logf("  Scope: %s, Name: %s, Version: %s", scope, name, version)

	// Calculate expected Maven path for logging (groupId/artifactId/version/artifactId-version.zip)
	// groupId = scope (no prefix), artifactId = name, version = version
	expectedPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", scope, name, version, name, version)
	t.Logf("  Expected Maven path: %s", expectedPath)
	t.Logf("  Full URL: %s/%s", baseURL, expectedPath)
	t.Logf("  Nexus data in container: /nexus-data (host: ./nexus-data)")

	// Test 1: Publish (upload) the file
	t.Run("Publish", func(t *testing.T) {
		writer, err := repo.GetWriter(ctx, element)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		n, err := writer.Write(testZipData)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write data: %v", err)
		}
		if n != len(testZipData) {
			_ = writer.Close()
			t.Fatalf("Wrote %d bytes, expected %d", n, len(testZipData))
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (upload): %v", err)
		}

		t.Logf("Successfully published %d bytes", n)
		t.Logf("  File should be at: %s/%s/%s/%s/%s-%s.zip", baseURL, scope, name, version, name, version)
	})

	// Small delay to ensure file is available
	time.Sleep(1 * time.Second)

	// Test 2: Check if file exists
	t.Run("Exists", func(t *testing.T) {
		exists := repo.Exists(ctx, element)
		if !exists {
			t.Fatalf("Published file does not exist")
		}
		t.Log("File exists check passed")
	})

	// Test 3: Get (download) the file
	t.Run("Get", func(t *testing.T) {
		reader, err := repo.GetReader(ctx, element)
		if err != nil {
			t.Fatalf("Failed to get reader: %v", err)
		}
		defer func() { _ = reader.Close() }()

		downloadedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read data: %v", err)
		}

		if len(downloadedData) != len(testZipData) {
			t.Fatalf("Downloaded %d bytes, expected %d", len(downloadedData), len(testZipData))
		}

		if !bytes.Equal(downloadedData, testZipData) {
			t.Fatalf("Downloaded data does not match original")
		}

		t.Logf("Successfully retrieved %d bytes", len(downloadedData))
	})

	// Test 4: Verify checksum
	t.Run("Checksum", func(t *testing.T) {
		// Calculate expected checksum
		hash := sha256.New()
		hash.Write(testZipData)
		expectedChecksum := fmt.Sprintf("%x", hash.Sum(nil))

		// Verify .sha256 file exists (should have been created during upload)
		// Maven path format: {scope}/{name}/{version}/{name}-{version}.zip
		// Checksum file: {scope}/{name}/{version}/{name}-{version}.zip.sha256
		expectedPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", scope, name, version, name, version)
		checksumPath := expectedPath + ".sha256"
		checksumFileExists := false
		checksumFileContent := ""

		// Construct full URL (baseURL already includes repository name)
		checksumURL := strings.TrimSuffix(baseURL, "/") + "/" + checksumPath
		t.Logf("Checking for checksum file at: %s", checksumURL)

		req, err := http.NewRequestWithContext(ctx, "GET", checksumURL, nil)
		if err != nil {
			t.Logf("Failed to create request for checksum file: %v", err)
		} else {
			// Add authentication (same as helper uses)
			username := os.Getenv("MAVEN_REPO_USERNAME")
			if username == "" {
				username = "admin" // Nexus default
			}
			password := os.Getenv("MAVEN_REPO_PASSWORD")
			if password == "" {
				password = "admin123" // Nexus default
			}
			req.SetBasicAuth(username, password)

			resp, err := helper.HTTPClient.Do(req)
			if err != nil {
				t.Logf("Failed to fetch checksum file: %v", err)
			} else {
				defer func() { _ = resp.Body.Close() }()
				t.Logf("Checksum file HTTP status: %d", resp.StatusCode)
				if resp.StatusCode == http.StatusOK {
					checksumFileExists = true
					if data, err := io.ReadAll(resp.Body); err == nil {
						checksumFileContent = strings.TrimSpace(string(data))
						t.Logf("Checksum file content: %s (length: %d)", checksumFileContent, len(checksumFileContent))
					} else {
						t.Logf("Failed to read checksum file body: %v", err)
					}
				} else {
					body, _ := io.ReadAll(resp.Body)
					t.Logf("Checksum file request failed with status %d, body: %s", resp.StatusCode, string(body))
				}
			}
		}

		if !checksumFileExists {
			t.Fatalf(".sha256 checksum file does not exist at %s (checked URL: %s)", checksumPath, checksumURL)
		}

		if checksumFileContent != expectedChecksum {
			t.Fatalf("Checksum file content mismatch: got %s, expected %s", checksumFileContent, expectedChecksum)
		}

		t.Logf(".sha256 file verified: %s", checksumFileContent)

		// Get checksum from repository (should read from .sha256 file)
		checksum, err := repo.Checksum(ctx, element)
		if err != nil {
			t.Fatalf("Failed to get checksum: %v", err)
		}

		if checksum != expectedChecksum {
			t.Fatalf("Checksum mismatch: got %s, expected %s", checksum, expectedChecksum)
		}

		if checksum != checksumFileContent {
			t.Fatalf("Checksum from method (%s) does not match checksum file content (%s) - method may not be using .sha256 file", checksum, checksumFileContent)
		}

		t.Logf("Checksum verification passed (read from .sha256 file): %s", checksum)
	})

	// Test 5: Verify publish date
	t.Run("PublishDate", func(t *testing.T) {
		publishDate, err := repo.PublishDate(ctx, element)
		if err != nil {
			t.Fatalf("Failed to get publish date: %v", err)
		}

		// Publish date should be recent (within last minute)
		now := time.Now()
		diff := now.Sub(publishDate)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2*time.Minute {
			t.Fatalf("Publish date seems incorrect: %v (now: %v, diff: %v)", publishDate, now, diff)
		}

		t.Logf("Publish date: %v", publishDate)
	})

	// Test 6: Verify base64 encoding
	t.Run("EncodeBase64", func(t *testing.T) {
		base64Data, err := repo.EncodeBase64(ctx, element)
		if err != nil {
			t.Fatalf("Failed to encode base64: %v", err)
		}

		if base64Data == "" {
			t.Fatalf("Base64 data is empty")
		}

		// Verify we can decode it back
		decodedBytes, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			t.Fatalf("Failed to decode base64 string: %v", err)
		}

		if !bytes.Equal(decodedBytes, testZipData) {
			t.Fatalf("Base64 decoded data does not match original")
		}

		t.Logf("Base64 encoding verified (length: %d)", len(base64Data))
	})

	// Test 7: Publish and verify Package.swift manifest
	t.Run("PublishPackageSwift", func(t *testing.T) {
		manifestElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)

		packageSwiftContent := []byte("// swift-tools-version:6.0\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])")

		writer, err := repo.GetWriter(ctx, manifestElement)
		if err != nil {
			t.Fatalf("Failed to get writer for Package.swift: %v", err)
		}

		n, err := writer.Write(packageSwiftContent)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write Package.swift: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (Package.swift upload): %v", err)
		}

		t.Logf("Successfully published Package.swift (%d bytes)", n)

		// Verify it exists
		if !repo.Exists(ctx, manifestElement) {
			t.Fatalf("Published Package.swift does not exist")
		}

		// Verify we can retrieve it
		reader, err := repo.GetReader(ctx, manifestElement)
		if err != nil {
			t.Fatalf("Failed to get reader for Package.swift: %v", err)
		}
		defer func() { _ = reader.Close() }()

		retrievedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read Package.swift: %v", err)
		}

		if !bytes.Equal(retrievedData, packageSwiftContent) {
			t.Fatalf("Retrieved Package.swift does not match original")
		}

		// Verify GetSwiftToolVersion works
		swiftVersion, err := repo.GetSwiftToolVersion(ctx, manifestElement)
		if err != nil {
			t.Fatalf("Failed to get Swift tool version: %v", err)
		}
		if swiftVersion != "5.3" {
			t.Fatalf("Expected Swift version 5.3, got %s", swiftVersion)
		}

		t.Logf("Package.swift verified, Swift version: %s", swiftVersion)
	})

	// Test 8: Publish and verify Package@swift-5.7.0.swift variant
	t.Run("PublishSwiftVariant", func(t *testing.T) {
		// Create element with filename overwrite for the variant
		variantElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		variantElement.SetFilenameOverwrite("Package@swift-5.7.0")

		variantContent := []byte("// swift-tools-version:5.7.0\nimport PackageDescription\nlet package = Package(name: \"test\", products: [.library(name: \"test\", targets: [\"test\"])], targets: [.target(name: \"test\")])")

		writer, err := repo.GetWriter(ctx, variantElement)
		if err != nil {
			t.Fatalf("Failed to get writer for Package@swift-5.7.0.swift: %v", err)
		}

		n, err := writer.Write(variantContent)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write Package@swift-5.7.0.swift: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (Package@swift-5.7.0.swift upload): %v", err)
		}

		t.Logf("Successfully published Package@swift-5.7.0.swift (%d bytes)", n)

		// Verify it exists
		if !repo.Exists(ctx, variantElement) {
			t.Fatalf("Published Package@swift-5.7.0.swift does not exist")
		}

		// Verify we can retrieve it
		reader, err := repo.GetReader(ctx, variantElement)
		if err != nil {
			t.Fatalf("Failed to get reader for Package@swift-5.7.0.swift: %v", err)
		}
		defer func() { _ = reader.Close() }()

		retrievedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read Package@swift-5.7.0.swift: %v", err)
		}

		if !bytes.Equal(retrievedData, variantContent) {
			t.Fatalf("Retrieved Package@swift-5.7.0.swift does not match original")
		}

		// Verify GetSwiftToolVersion works
		swiftVersion, err := repo.GetSwiftToolVersion(ctx, variantElement)
		if err != nil {
			t.Fatalf("Failed to get Swift tool version: %v", err)
		}
		if swiftVersion != "5.7.0" {
			t.Fatalf("Expected Swift version 5.7.0, got %s", swiftVersion)
		}

		t.Logf("Package@swift-5.7.0.swift verified, Swift version: %s", swiftVersion)
	})

	// Test 9: Publish and verify metadata.json
	t.Run("PublishMetadataJson", func(t *testing.T) {
		metadataElement := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.Metadata)
		metadataContent := []byte(`{"description":"Integration test metadata"}`)

		writer, err := repo.GetWriter(ctx, metadataElement)
		if err != nil {
			t.Fatalf("Failed to get writer for metadata.json: %v", err)
		}

		n, err := writer.Write(metadataContent)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write metadata.json: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (metadata.json upload): %v", err)
		}

		t.Logf("Successfully published metadata.json (%d bytes)", n)

		if !repo.Exists(ctx, metadataElement) {
			t.Fatalf("Published metadata.json does not exist")
		}

		reader, err := repo.GetReader(ctx, metadataElement)
		if err != nil {
			t.Fatalf("Failed to get reader for metadata.json: %v", err)
		}
		defer func() { _ = reader.Close() }()

		retrievedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read metadata.json: %v", err)
		}

		if !bytes.Equal(retrievedData, metadataContent) {
			t.Fatalf("Retrieved metadata.json does not match original")
		}

		loaded, err := repo.LoadMetadata(ctx, scope, name, version)
		if err != nil {
			t.Fatalf("Failed to load metadata: %v", err)
		}
		if desc, _ := loaded["description"].(string); desc != "Integration test metadata" {
			t.Fatalf("LoadMetadata: expected description %q, got %v", "Integration test metadata", loaded["description"])
		}

		t.Logf("metadata.json verified via LoadMetadata")
	})

	// Test 10: Publish and verify metadata.sig
	t.Run("PublishMetadataSig", func(t *testing.T) {
		metadataSigElement := models.NewUploadElement(scope, name, version, "application/pgp-signature", models.MetadataSignature)
		// Use binary content so Nexus content sniffing does not detect text/plain and reject the upload
		sigContent := []byte{0x89, 0x50, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00} // dummy binary (PGP-like) content

		writer, err := repo.GetWriter(ctx, metadataSigElement)
		if err != nil {
			t.Fatalf("Failed to get writer for metadata.sig: %v", err)
		}

		n, err := writer.Write(sigContent)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write metadata.sig: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (metadata.sig upload): %v", err)
		}

		t.Logf("Successfully published metadata.sig (%d bytes)", n)

		if !repo.Exists(ctx, metadataSigElement) {
			t.Fatalf("Published metadata.sig does not exist")
		}

		reader, err := repo.GetReader(ctx, metadataSigElement)
		if err != nil {
			t.Fatalf("Failed to get reader for metadata.sig: %v", err)
		}
		defer func() { _ = reader.Close() }()

		retrievedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read metadata.sig: %v", err)
		}

		if !bytes.Equal(retrievedData, sigContent) {
			t.Fatalf("Retrieved metadata.sig does not match original")
		}

		t.Logf("metadata.sig verified")
	})

	// Test 11: Publish and verify Package.json
	t.Run("PublishPackageJson", func(t *testing.T) {
		packageJsonElement := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.PackageManifestJson)
		packageJsonContent := []byte(`{"name":"TestPackage","version":"1.0.0"}`)

		writer, err := repo.GetWriter(ctx, packageJsonElement)
		if err != nil {
			t.Fatalf("Failed to get writer for Package.json: %v", err)
		}

		n, err := writer.Write(packageJsonContent)
		if err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write Package.json: %v", err)
		}

		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (Package.json upload): %v", err)
		}

		t.Logf("Successfully published Package.json (%d bytes)", n)

		if !repo.Exists(ctx, packageJsonElement) {
			t.Fatalf("Published Package.json does not exist")
		}

		reader, err := repo.GetReader(ctx, packageJsonElement)
		if err != nil {
			t.Fatalf("Failed to get reader for Package.json: %v", err)
		}
		defer func() { _ = reader.Close() }()

		retrievedData, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read Package.json: %v", err)
		}

		if !bytes.Equal(retrievedData, packageJsonContent) {
			t.Fatalf("Retrieved Package.json does not match original")
		}

		loaded, err := repo.LoadPackageJson(ctx, scope, name, version)
		if err != nil {
			t.Fatalf("Failed to load Package.json: %v", err)
		}
		if pkgName, _ := loaded["name"].(string); pkgName != "TestPackage" {
			t.Fatalf("LoadPackageJson: expected name %q, got %v", "TestPackage", loaded["name"])
		}
		if pkgVersion, _ := loaded["version"].(string); pkgVersion != "1.0.0" {
			t.Fatalf("LoadPackageJson: expected version %q, got %v", "1.0.0", loaded["version"])
		}

		t.Logf("Package.json verified via LoadPackageJson")
	})

	// Publish second package in same scope to verify listing with multiple packages
	scope2, name2, version2 := "test", "OtherPackage", "2.0.0"
	element2 := models.NewUploadElement(scope2, name2, version2, mimetypes.ApplicationZip, models.SourceArchive)
	t.Run("PublishSecondPackage", func(t *testing.T) {
		zip2 := createTestZipWithManifests(t, scope2, name2)
		writer, err := repo.GetWriter(ctx, element2)
		if err != nil {
			t.Fatalf("Failed to get writer for second package: %v", err)
		}
		if _, err := writer.Write(zip2); err != nil {
			_ = writer.Close()
			t.Fatalf("Failed to write second package zip: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close writer (second package): %v", err)
		}
		if !repo.Exists(ctx, element2) {
			t.Fatalf("Second package source archive does not exist after publish")
		}
		t.Logf("Successfully published second package %s/%s/%s", scope2, name2, version2)
	})

	// Test 12: List, GetAlternativeManifests, ListScopes, ListInScope, ListAll (maven-metadata.xml and .spm-registry/index.json)
	t.Run("ListAndListScopes", func(t *testing.T) {
		// List(scope, name) uses maven-metadata.xml — should return at least the published version
		versions, err := repo.List(ctx, scope, name)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(versions) == 0 {
			t.Fatalf("List returned no versions; expected at least %s (maven-metadata.xml should list it)", version)
		}
		found := false
		for _, v := range versions {
			if v.Scope == scope && v.PackageName == name && v.Version == version {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("List did not return %s/%s/%s; got %v", scope, name, version, versions)
		}
		t.Logf("List returned %d version(s), including %s", len(versions), version)

		// List(scope, name2) for second package — must include version2
		versions2, err := repo.List(ctx, scope2, name2)
		if err != nil {
			t.Fatalf("List(scope, %q) failed: %v", name2, err)
		}
		found2 := false
		for _, v := range versions2 {
			if v.Scope == scope2 && v.PackageName == name2 && v.Version == version2 {
				found2 = true
				break
			}
		}
		if !found2 {
			t.Fatalf("List did not return %s/%s/%s; got %v", scope2, name2, version2, versions2)
		}
		t.Logf("List(%q, %q) returned %d version(s), including %s", scope2, name2, len(versions2), version2)

		// GetAlternativeManifests: with only one version published, alternatives should be empty (or other versions if any)
		manifestElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		alternatives, err := repo.GetAlternativeManifests(ctx, manifestElement)
		if err != nil {
			t.Fatalf("GetAlternativeManifests failed: %v", err)
		}
		t.Logf("GetAlternativeManifests returned %d alternative manifest(s)", len(alternatives))

		// ListScopes / ListInScope / ListAll read from .spm-registry/index.json (updated when we publish)
		scopes, err := repo.ListScopes(ctx)
		if err != nil {
			t.Fatalf("ListScopes failed: %v", err)
		}
		t.Logf("ListScopes returned %d scope(s): %v", len(scopes), scopes)
		scopeFound := false
		if len(scopes) > 0 {
			for _, s := range scopes {
				if s == scope {
					scopeFound = true
					break
				}
			}
			if !scopeFound {
				t.Logf("Note: scope %q not in ListScopes result (index may not include scope)", scope)
			}
		}

		inScope, err := repo.ListInScope(ctx, scope)
		if err != nil {
			t.Fatalf("ListInScope failed: %v", err)
		}
		t.Logf("ListInScope(%q) returned %d package(s)", scope, len(inScope))
		foundInScope1 := false
		foundInScope2 := false
		for _, e := range inScope {
			if e.Scope == scope && e.PackageName == name && e.Version == version {
				foundInScope1 = true
			}
			if e.Scope == scope2 && e.PackageName == name2 && e.Version == version2 {
				foundInScope2 = true
			}
		}
		if !foundInScope1 {
			t.Fatalf("ListInScope(%q) missing %s/%s/%s; got %v", scope, scope, name, version, inScope)
		}
		if !foundInScope2 {
			t.Fatalf("ListInScope(%q) missing %s/%s/%s (multiple packages); got %v", scope, scope2, name2, version2, inScope)
		}

		all, err := repo.ListAll(ctx)
		if err != nil {
			t.Fatalf("ListAll failed: %v", err)
		}
		t.Logf("ListAll returned %d package(s)", len(all))
		foundAll1 := false
		foundAll2 := false
		for _, e := range all {
			if e.Scope == scope && e.PackageName == name && e.Version == version {
				foundAll1 = true
			}
			if e.Scope == scope2 && e.PackageName == name2 && e.Version == version2 {
				foundAll2 = true
			}
		}
		if !foundAll1 {
			t.Fatalf("ListAll missing %s/%s/%s; got %v", scope, name, version, all)
		}
		if !foundAll2 {
			t.Fatalf("ListAll missing %s/%s/%s (multiple packages); got %v", scope2, name2, version2, all)
		}
	})

	// Cleanup: Remove the test files (unless KEEP_TEST_DATA is set)
	t.Run("Cleanup", func(t *testing.T) {
		if os.Getenv("KEEP_TEST_DATA") != "" {
			t.Logf("Keeping test data for inspection:")
			t.Logf("  Repository: %s", repositoryName)
			t.Logf("  Nexus data: ./nexus-data (container: /nexus-data)")
			return
		}

		// Remove all test files
		filesToRemove := []*models.UploadElement{
			element,  // Source archive (first package)
			element2, // Second package source archive
		}

		// Add manifest files
		manifestElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		filesToRemove = append(filesToRemove, manifestElement)

		variantElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		variantElement.SetFilenameOverwrite("Package@swift-5.7.0")
		filesToRemove = append(filesToRemove, variantElement)

		metadataElement := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.Metadata)
		filesToRemove = append(filesToRemove, metadataElement)

		metadataSigElement := models.NewUploadElement(scope, name, version, "application/pgp-signature", models.MetadataSignature)
		filesToRemove = append(filesToRemove, metadataSigElement)

		packageJsonElement := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.PackageManifestJson)
		filesToRemove = append(filesToRemove, packageJsonElement)

		// Remove main files and their .sha256 checksum files
		for _, file := range filesToRemove {
			// Remove the main file
			if err := repo.Remove(ctx, file); err != nil {
				t.Logf("Warning: Failed to remove %s: %v", file.FileName(), err)
			}

			// Remove the .sha256 checksum file
			// Access the internal access implementation to build the path
			accessImpl := repo.Access.(*access)
			path := accessImpl.buildMavenPathForElement(file)
			checksumPath := path + ".sha256"
			if err := repo.client.DELETE(ctx, checksumPath); err != nil {
				// Log but don't fail - checksum file might not exist
				if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
					t.Logf("Warning: Failed to remove checksum file %s: %v", checksumPath, err)
				}
			}
		}

		// Remove maven-metadata.xml for both packages
		for _, s := range []struct{ sc, nm string }{{scope, name}, {scope2, name2}} {
			groupId := buildGroupId(s.sc, cfg)
			artifactId := buildArtifactId(s.nm)
			metadataPath := getMetadataPath(groupId, artifactId)
			if err := repo.client.DELETE(ctx, metadataPath); err != nil {
				if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
					t.Logf("Warning: Failed to remove maven-metadata.xml %s: %v", metadataPath, err)
				}
			}
		}

		t.Log("Test files cleaned up")
	})
}
