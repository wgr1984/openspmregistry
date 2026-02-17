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
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// MavenRepo implements the repo.Repo interface for Maven repositories
type MavenRepo struct {
	repo.Access
	client       *client
	config       config.MavenConfig
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
	mediaType, _, _ := mime.ParseMediaType(element.MimeType)
	if mediaType != mimetypes.ApplicationZip {
		return errors.New("unsupported mime type")
	}

	// Download source archive
	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return fmt.Errorf("failed to get source archive: %w", err)
	}
	defer func() { _ = reader.Close() }()

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
		defer func() { _ = r.Close() }()

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
			_ = writer.Close()
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
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

// PublishDate returns the publish date from Last-Modified header
func (m *MavenRepo) PublishDate(ctx context.Context, element *models.UploadElement) (time.Time, error) {
	path := m.Access.(*access).buildMavenPathForElement(element)

	resp, err := m.client.HEAD(ctx, path)
	if err != nil {
		return m.timeProvider.Now(), err
	}
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = reader.Close() }()

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
// First tries to read from the .sha256 checksum file (Maven convention)
// Falls back to calculating the checksum if the file doesn't exist
func (m *MavenRepo) Checksum(ctx context.Context, element *models.UploadElement) (string, error) {
	if !m.Exists(ctx, element) {
		return "", fmt.Errorf("file does not exist: %s", element.FileName())
	}

	// Try to read from .sha256 checksum file first (more efficient)
	path := m.Access.(*access).buildMavenPathForElement(element)

	checksumPath := path + ".sha256"
	resp, err := m.client.GET(ctx, checksumPath)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode == http.StatusOK {
			checksumBytes, err := io.ReadAll(resp.Body)
			if err == nil {
				checksum := strings.TrimSpace(string(checksumBytes))
				// Validate it's a valid hex string (64 chars for SHA256)
				if len(checksum) == 64 {
					if slog.Default().Enabled(ctx, slog.LevelDebug) {
						slog.Debug("Checksum read from .sha256 file", "path", checksumPath, "checksum", checksum)
					}
					return checksum, nil
				}
			}
		}
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("Checksum file not found or invalid, calculating from artifact", "path", checksumPath, "error", err)
	}

	// Fall back to calculating checksum from artifact
	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// GetAlternativeManifests returns manifest variants suitable for Link rel=alternate (same-version
// swift-version only, e.g. Package@swift-5.8.swift). From maven-metadata we only get other package
// versions, each with default Package.swift; those are not Link alternates, so we return none.
func (m *MavenRepo) GetAlternativeManifests(ctx context.Context, element *models.UploadElement) ([]models.UploadElement, error) {
	groupId := buildGroupId(element.Scope, m.config)
	artifactId := buildArtifactId(element.Name)
	metadata, err := loadMetadata(m.client, ctx, groupId, artifactId)
	if err != nil {
		return []models.UploadElement{}, nil
	}
	var out []models.UploadElement
	for _, v := range metadata.Versioning.Versions.Version {
		if v == element.Version {
			continue
		}
		manifest := models.NewUploadElement(element.Scope, element.Name, v, mimetypes.TextXSwift, models.Manifest).SetFilenameOverwrite("Package")
		// Only include entries that are Link-eligible (swift-version variants, not default Package.swift).
		if manifest.FileName() != "Package.swift" {
			out = append(out, *manifest)
		}
	}
	return out, nil
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
	defer func() { _ = reader.Close() }()

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
	path := m.Access.(*access).buildMavenPathForElement(element)

	return m.client.DELETE(ctx, path)
}

// ListScopes returns all available scopes from .spm-registry/index.json only.
// If the index is missing or invalid, returns an empty list and nil error (no directory/HTML fallback).
func (m *MavenRepo) ListScopes(ctx context.Context) ([]string, error) {
	scopes, err := m.client.getSPMRegistryIndex(ctx)
	if err != nil || scopes == nil {
		return []string{}, nil
	}
	return scopes, nil
}

// ListInScope returns all packages in a scope from .spm-registry/index.json only.
// Package names come from index.packages[scope]; versions from maven-metadata.xml per package.
// If the index is missing or has no packages for the scope, returns an empty list (no fallback).
func (m *MavenRepo) ListInScope(ctx context.Context, scope string) ([]models.ListElement, error) {
	index, err := m.client.getSPMRegistryIndexFull(ctx)
	if err != nil || index == nil || index.Packages == nil {
		return []models.ListElement{}, nil
	}
	artifactIds := index.Packages[scope]
	if len(artifactIds) == 0 {
		return []models.ListElement{}, nil
	}
	groupId := buildGroupId(scope, m.config)
	var out []models.ListElement
	for _, artifactId := range artifactIds {
		metadata, err := loadMetadata(m.client, ctx, groupId, artifactId)
		if err != nil {
			continue
		}
		for _, v := range metadata.Versioning.Versions.Version {
			out = append(out, *models.NewListElement(scope, artifactId, v))
		}
	}
	return out, nil
}

// ListAll returns all packages across all scopes.
func (m *MavenRepo) ListAll(ctx context.Context) ([]models.ListElement, error) {
	scopes, err := m.ListScopes(ctx)
	if err != nil {
		return nil, err
	}
	var all []models.ListElement
	for _, scope := range scopes {
		packages, err := m.ListInScope(ctx, scope)
		if err != nil {
			slog.Warn("Error listing packages in scope", "scope", scope, "error", err)
			continue
		}
		all = append(all, packages...)
	}
	return all, nil
}

// LoadPackageJson loads Package.json sidecar
func (m *MavenRepo) LoadPackageJson(ctx context.Context, scope string, name string, version string) (map[string]any, error) {
	element := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.PackageManifestJson)
	if !m.Exists(ctx, element) {
		return nil, fmt.Errorf("package.json not found")
	}

	reader, err := m.GetReader(ctx, element)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	var packageJson map[string]any
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&packageJson); err != nil {
		return nil, fmt.Errorf("failed to parse Package.json: %w", err)
	}

	return packageJson, nil
}
