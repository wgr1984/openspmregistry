package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/repo/files"
	"OpenSPMRegistry/utils"
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// MavenRepo implements the repo.Repo interface for Maven repositories
type MavenRepo struct {
	repo.Access
	client      *client
	config      config.MavenConfig
	timeProvider utils.TimeProvider
}

// NewMavenRepo creates a new Maven repository instance
func NewMavenRepo(cfg config.MavenConfig) (*MavenRepo, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Maven client: %w", err)
	}

	access := newAccess(client, cfg)

	return &MavenRepo{
		Access:       access,
		client:       client,
		config:       cfg,
		timeProvider: utils.NewRealTimeProvider(),
	}, nil
}

// ExtractManifestFiles extracts Package.swift and Package.json from source archive
func (m *MavenRepo) ExtractManifestFiles(ctx context.Context, element *models.UploadElement) error {
	if element.MimeType != mimetypes.ApplicationZip {
		return errors.New("unsupported mime type")
	}

	// Download source archive
	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return fmt.Errorf("failed to get source archive: %w", err)
	}
	defer reader.Close()

	// Read source archive into memory (SPM packages are typically small)
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read source archive: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}

	// Use shared extraction logic with Maven-specific upload handler
	fileExtractor := func(name string, r io.ReadCloser) error {
		defer r.Close()

		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)

		// Determine element type based on filename
		var manifestElement *models.UploadElement

		if strings.HasPrefix(strings.ToLower(name), "package") && strings.ToLower(ext) == ".swift" {
			manifestElement = models.NewUploadElement(element.Scope, element.Name, element.Version, mimetypes.TextXSwift, models.Manifest)
			manifestElement.SetFilenameOverwrite(base)
		} else if strings.ToLower(name) == "package.json" {
			manifestElement = models.NewUploadElement(element.Scope, element.Name, element.Version, mimetypes.ApplicationJson, models.PackageManifestJson)
		} else {
			// Skip unexpected files
			return nil
		}

		writer, err := m.GetWriter(ctx, manifestElement)
		if err != nil {
			slog.Warn("Failed to get writer for manifest", "manifest", name, "error", err)
			return nil
		}

		if _, err := writer.Write(data); err != nil {
			writer.Close()
			slog.Warn("Failed to write manifest", "manifest", name, "error", err)
			return nil
		}

		if err := writer.Close(); err != nil {
			slog.Warn("Failed to upload manifest", "manifest", name, "error", err)
			return nil // Don't fail the entire extraction on upload errors
		}

		return nil
	}

	return files.ExtractManifestFilesFromZipReader(element, zipReader, fileExtractor)
}

// List returns all versions of a package
func (m *MavenRepo) List(ctx context.Context, scope string, name string) ([]models.ListElement, error) {
	groupId := buildGroupId(scope, m.config)
	artifactId := buildArtifactId(name)

	metadata, err := loadMetadata(m.client, ctx, groupId, artifactId)
	if err != nil {
		// If metadata doesn't exist, return empty list
		return []models.ListElement{}, nil
	}

	var elements []models.ListElement
	for _, version := range metadata.Versioning.Versions.Version {
		elements = append(elements, *models.NewListElement(scope, name, version))
	}

	return elements, nil
}

// EncodeBase64 returns the base64 representation of the content
func (m *MavenRepo) EncodeBase64(ctx context.Context, element *models.UploadElement) (string, error) {
	if !m.Exists(ctx, element) {
		return "", fmt.Errorf("file does not exist: %s", element.FileName())
	}

	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// PublishDate returns the publish date from Last-Modified header
func (m *MavenRepo) PublishDate(ctx context.Context, element *models.UploadElement) (time.Time, error) {
	path, err := m.Access.(*access).buildMavenPathForElement(element)
	if err != nil {
		return m.timeProvider.Now(), err
	}

	resp, err := m.client.HEAD(ctx, path)
	if err != nil {
		return m.timeProvider.Now(), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return m.timeProvider.Now(), fmt.Errorf("element not found")
	}

	lastModified := resp.Header.Get("Last-Modified")
	if lastModified != "" {
		t, err := time.Parse(time.RFC1123, lastModified)
		if err != nil {
			// Try RFC1123Z
			t, err = time.Parse(time.RFC1123Z, lastModified)
			if err != nil {
				return m.timeProvider.Now(), err
			}
		}
		return t, nil
	}

	return m.timeProvider.Now(), nil
}

// LoadMetadata loads the metadata.json sidecar
func (m *MavenRepo) LoadMetadata(ctx context.Context, scope string, name string, version string) (map[string]any, error) {
	element := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.Metadata)
	if !m.Exists(ctx, element) {
		return nil, fmt.Errorf("metadata not found")
	}

	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return metadata, nil
}

// Checksum computes SHA256 checksum of the element
func (m *MavenRepo) Checksum(ctx context.Context, element *models.UploadElement) (string, error) {
	if !m.Exists(ctx, element) {
		return "", fmt.Errorf("file does not exist: %s", element.FileName())
	}

	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// GetAlternativeManifests returns alternative Package.swift versions
func (m *MavenRepo) GetAlternativeManifests(ctx context.Context, element *models.UploadElement) ([]models.UploadElement, error) {
	// For Maven, we need to check for sidecar files with different names
	// This is limited - we can only find manifests we've uploaded
	// In practice, we'd need to list the directory or use a different approach
	
	// For now, return empty - this would require directory listing which Maven doesn't easily support
	// A better approach would be to maintain a manifest index
	return []models.UploadElement{}, nil
}

// GetSwiftToolVersion extracts swift-tools-version from Package.swift
func (m *MavenRepo) GetSwiftToolVersion(ctx context.Context, manifest *models.UploadElement) (string, error) {
	if !m.Exists(ctx, manifest) {
		return "", fmt.Errorf("manifest does not exist: %s", manifest.FileName())
	}

	reader, err := m.GetReader(ctx, manifest)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	const swiftVersionPrefix = "// swift-tools-version:"
	scanner := bufio.NewScanner(reader)
	if scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, swiftVersionPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, swiftVersionPrefix)), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", errors.New("swift-tools-version not found")
}

// Lookup finds packages by repository URL (not well supported by Maven)
func (m *MavenRepo) Lookup(ctx context.Context, url string) []string {
	// Maven doesn't support this directly
	// Would need to scan all metadata files, which is expensive
	// Return empty for now
	return []string{}
}

// Remove deletes an element from the Maven repository
func (m *MavenRepo) Remove(ctx context.Context, element *models.UploadElement) error {
	path, err := m.Access.(*access).buildMavenPathForElement(element)
	if err != nil {
		return err
	}

	return m.client.DELETE(ctx, path)
}

// ListScopes returns all available scopes (limited by Maven structure)
func (m *MavenRepo) ListScopes(ctx context.Context) ([]string, error) {
	// Maven doesn't easily support listing all groupIds
	// This would require directory listing which may not be available
	// Return empty for now - this is a limitation of Maven repositories
	return []string{}, nil
}

// ListInScope returns all packages in a scope
func (m *MavenRepo) ListInScope(ctx context.Context, scope string) ([]models.ListElement, error) {
	// Similar limitation - would need to list groupId directory
	// Return empty for now
	return []models.ListElement{}, nil
}

// ListAll returns all packages (very limited for Maven)
func (m *MavenRepo) ListAll(ctx context.Context) ([]models.ListElement, error) {
	// This is not practical for Maven repositories without directory listing
	return []models.ListElement{}, nil
}

// LoadPackageJson loads Package.json sidecar
func (m *MavenRepo) LoadPackageJson(ctx context.Context, scope string, name string, version string) (map[string]any, error) {
	element := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.PackageManifestJson)
	if !m.Exists(ctx, element) {
		return nil, fmt.Errorf("Package.json not found")
	}

	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var packageJson map[string]any
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&packageJson); err != nil {
		return nil, fmt.Errorf("failed to parse Package.json: %w", err)
	}

	return packageJson, nil
}
