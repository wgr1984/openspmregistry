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
			req, err := http.NewRequestWithContext(ctx, "GET", h.BaseURL, nil)
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
// It reads credentials from environment variables or uses Archiva defaults
func (h *IntegrationTestHelper) GetMavenConfig() config.MavenConfig {
	username := os.Getenv("MAVEN_REPO_USERNAME")
	if username == "" {
		username = "admin" // Reposilite default
	}

	password := os.Getenv("MAVEN_REPO_PASSWORD")
	if password == "" {
		password = "admin123" // Reposilite default
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
	packageSwift.Write([]byte("// swift-tools-version:5.3\nlet package = Package(name: \"test\")"))

	// Add Package@swift-5.7.0.swift variant
	packageSwift57, err := zipWriter.Create(dirPrefix + "Package@swift-5.7.0.swift")
	if err != nil {
		t.Fatalf("Failed to create Package@swift-5.7.0.swift in zip: %v", err)
	}
	packageSwift57.Write([]byte("// swift-tools-version:5.7.0\nlet package = Package(name: \"test\")"))

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
		baseURL = "http://localhost:8080"
	}

	// Reposilite requires a repository name prefix (e.g., "private")
	// If BaseURL doesn't already include a repository path, append the default repository
	repositoryName := os.Getenv("MAVEN_REPO_NAME")
	if repositoryName == "" {
		repositoryName = "private" // Reposilite default repository
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
	t.Logf("  Local file path: ./maven-files/repositories/%s/%s", repositoryName, expectedPath)

	// Test 1: Publish (upload) the file
	t.Run("Publish", func(t *testing.T) {
		writer, err := repo.GetWriter(ctx, element)
		if err != nil {
			t.Fatalf("Failed to get writer: %v", err)
		}

		n, err := writer.Write(testZipData)
		if err != nil {
			writer.Close()
			t.Fatalf("Failed to write data: %v", err)
		}
		if n != len(testZipData) {
			writer.Close()
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
		defer reader.Close()

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
				username = "admin" // Reposilite default
			}
			password := os.Getenv("MAVEN_REPO_PASSWORD")
			if password == "" {
				password = "admin123" // Reposilite default
			}
			req.SetBasicAuth(username, password)

			resp, err := helper.HTTPClient.Do(req)
			if err != nil {
				t.Logf("Failed to fetch checksum file: %v", err)
			} else {
				defer resp.Body.Close()
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

		packageSwiftContent := []byte("// swift-tools-version:5.3\nlet package = Package(name: \"test\")")

		writer, err := repo.GetWriter(ctx, manifestElement)
		if err != nil {
			t.Fatalf("Failed to get writer for Package.swift: %v", err)
		}

		n, err := writer.Write(packageSwiftContent)
		if err != nil {
			writer.Close()
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
		defer reader.Close()

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

		variantContent := []byte("// swift-tools-version:5.7.0\nlet package = Package(name: \"test\")")

		writer, err := repo.GetWriter(ctx, variantElement)
		if err != nil {
			t.Fatalf("Failed to get writer for Package@swift-5.7.0.swift: %v", err)
		}

		n, err := writer.Write(variantContent)
		if err != nil {
			writer.Close()
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
		defer reader.Close()

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

	// Cleanup: Remove the test files (unless KEEP_TEST_DATA is set)
	t.Run("Cleanup", func(t *testing.T) {
		if os.Getenv("KEEP_TEST_DATA") != "" {
			expectedPath := fmt.Sprintf("%s/%s/%s/%s-%s.zip", scope, name, version, name, version)
			packageSwiftPath := fmt.Sprintf("%s/%s/%s/%s-%s-package.swift", scope, name, version, name, version)
			variantPath := fmt.Sprintf("%s/%s/%s/%s-%s-package-swift-5.7.0.swift", scope, name, version, name, version)
			t.Logf("Keeping test data for inspection:")
			t.Logf("  Repository: %s", repositoryName)
			t.Logf("  Source archive: ./maven-files/repositories/%s/%s", repositoryName, expectedPath)
			t.Logf("  Package.swift: ./maven-files/repositories/%s/%s", repositoryName, packageSwiftPath)
			t.Logf("  Package@swift-5.7.0.swift: ./maven-files/repositories/%s/%s", repositoryName, variantPath)
			t.Logf("  Inspect with: ls -la ./maven-files/repositories/%s/%s/", repositoryName, fmt.Sprintf("%s/%s/%s", scope, name, version))
			return
		}

		// Remove all test files
		filesToRemove := []*models.UploadElement{
			element, // Source archive
		}

		// Add manifest files
		manifestElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		filesToRemove = append(filesToRemove, manifestElement)

		variantElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
		variantElement.SetFilenameOverwrite("Package@swift-5.7.0")
		filesToRemove = append(filesToRemove, variantElement)

		// Remove main files and their .sha256 checksum files
		for _, file := range filesToRemove {
			// Remove the main file
			if err := repo.Remove(ctx, file); err != nil {
				t.Logf("Warning: Failed to remove %s: %v", file.FileName(), err)
			}

			// Remove the .sha256 checksum file
			// Access the internal access implementation to build the path
			accessImpl := repo.Access.(*access)
			path, err := accessImpl.buildMavenPathForElement(file)
			if err == nil {
				checksumPath := path + ".sha256"
				if err := repo.client.DELETE(ctx, checksumPath); err != nil {
					// Log but don't fail - checksum file might not exist
					if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
						t.Logf("Warning: Failed to remove checksum file %s: %v", checksumPath, err)
					}
				}
			}
		}

		// Remove maven-metadata.xml file
		groupId := buildGroupId(scope, cfg)
		artifactId := buildArtifactId(name)
		metadataPath := getMetadataPath(groupId, artifactId)
		if err := repo.client.DELETE(ctx, metadataPath); err != nil {
			// Log but don't fail - metadata file might not exist
			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				t.Logf("Warning: Failed to remove maven-metadata.xml: %v", err)
			}
		}

		t.Log("Test files cleaned up")
	})
}
