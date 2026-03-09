package maven

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"OpenSPMRegistry/models"
)

// MavenMetadata represents a Maven metadata.xml structure
type MavenMetadata struct {
	XMLName    xml.Name   `xml:"metadata"`
	GroupId    string     `xml:"groupId"`
	ArtifactId string     `xml:"artifactId"`
	Versioning Versioning `xml:"versioning"`
}

// Versioning contains version information
type Versioning struct {
	Latest      string   `xml:"latest"`
	Release     string   `xml:"release"`
	Versions    Versions `xml:"versions"`
	LastUpdated string   `xml:"lastUpdated"`
}

// Versions contains a list of version strings
type Versions struct {
	Version []string `xml:"version"`
}

// ErrMetadataNotFound is returned by loadMetadata when maven-metadata.xml does not exist (HTTP 404).
// Callers should use errors.Is(err, ErrMetadataNotFound) to detect not-found and create new metadata only in that case.
var ErrMetadataNotFound = errors.New("maven metadata not found")

// parseMetadata parses a Maven metadata.xml file
func parseMetadata(data []byte) (*MavenMetadata, error) {
	var metadata MavenMetadata
	if err := xml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.xml: %w", err)
	}
	return &metadata, nil
}

// getMetadataPath returns the path to the Maven metadata.xml file
func getMetadataPath(groupId, artifactId string) string {
	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	return fmt.Sprintf("%s/%s/maven-metadata.xml", groupIdPath, artifactId)
}

// loadMetadata loads and parses a Maven metadata.xml file.
// Returns ErrMetadataNotFound when the file does not exist (HTTP 404); other errors (e.g. timeouts, 5xx, auth)
// are returned as-is so callers can avoid overwriting existing metadata on transient failures.
func loadMetadata(ctx context.Context, client *client, groupId, artifactId string) (*MavenMetadata, error) {
	path := getMetadataPath(groupId, artifactId)
	resp, err := client.GET(ctx, path)
	if err != nil {
		var statusErr *HTTPStatusError
		if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
			return nil, ErrMetadataNotFound
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	return parseMetadata(data)
}

// updateMetadata adds or updates a version in the metadata and uploads it.
// Creates new metadata only when maven-metadata.xml is missing (404); on other load errors (e.g. timeouts, 5xx)
// it returns the error without overwriting, to avoid losing existing versions.
func updateMetadata(ctx context.Context, client *client, groupId, artifactId, version string) error {
	metadata, err := loadMetadata(ctx, client, groupId, artifactId)
	if err != nil {
		if !errors.Is(err, ErrMetadataNotFound) {
			return err
		}
		// Metadata doesn't exist (404), create new one
		metadata = &MavenMetadata{
			GroupId:    groupId,
			ArtifactId: artifactId,
			Versioning: Versioning{
				Versions: Versions{
					Version: []string{version},
				},
			},
		}
		metadata.Versioning.Latest = version
		metadata.Versioning.Release = version
	} else {
		// Add version if it doesn't exist
		if !slices.Contains(metadata.Versioning.Versions.Version, version) {
			metadata.Versioning.Versions.Version = append(metadata.Versioning.Versions.Version, version)
		}

		// Sort versions by semantic order, highest first (models.SortVersions is descending)
		models.SortVersions(metadata.Versioning.Versions.Version)

		// Update latest and release to the highest version (first element after sort)
		if len(metadata.Versioning.Versions.Version) > 0 {
			latest := metadata.Versioning.Versions.Version[0]
			metadata.Versioning.Latest = latest
			metadata.Versioning.Release = latest
		}
	}

	// Update lastUpdated timestamp (format: yyyyMMddHHmmss)
	metadata.Versioning.LastUpdated = time.Now().Format("20060102150405")

	// Marshal to XML
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	// Upload metadata
	path := getMetadataPath(groupId, artifactId)
	reader := bytes.NewReader(buf.Bytes())
	return client.PUT(ctx, path, reader, "application/xml")
}
