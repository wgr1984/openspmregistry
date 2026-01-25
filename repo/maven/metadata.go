package maven

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// MavenMetadata represents a Maven metadata.xml structure
type MavenMetadata struct {
	XMLName    xml.Name `xml:"metadata"`
	GroupId    string   `xml:"groupId"`
	ArtifactId string   `xml:"artifactId"`
	Versioning Versioning `xml:"versioning"`
}

// Versioning contains version information
type Versioning struct {
	Latest     string   `xml:"latest"`
	Release    string   `xml:"release"`
	Versions   Versions `xml:"versions"`
	LastUpdated string  `xml:"lastUpdated"`
}

// Versions contains a list of version strings
type Versions struct {
	Version []string `xml:"version"`
}

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

// loadMetadata loads and parses a Maven metadata.xml file
func loadMetadata(client *client, ctx context.Context, groupId, artifactId string) (*MavenMetadata, error) {
	path := getMetadataPath(groupId, artifactId)
	resp, err := client.GET(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	return parseMetadata(data)
}

// updateMetadata adds or updates a version in the metadata and uploads it
// If metadata doesn't exist, it creates a new one
func updateMetadata(client *client, ctx context.Context, groupId, artifactId, version string) error {
	// Try to load existing metadata
	metadata, err := loadMetadata(client, ctx, groupId, artifactId)
	if err != nil {
		// Metadata doesn't exist, create new one
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
		// Check if version already exists
		versionExists := false
		for _, v := range metadata.Versioning.Versions.Version {
			if v == version {
				versionExists = true
				break
			}
		}

		// Add version if it doesn't exist
		if !versionExists {
			metadata.Versioning.Versions.Version = append(metadata.Versioning.Versions.Version, version)
		}

		// Sort versions (Maven typically expects sorted versions)
		sort.Strings(metadata.Versioning.Versions.Version)

		// Update latest and release to the highest version
		if len(metadata.Versioning.Versions.Version) > 0 {
			latest := metadata.Versioning.Versions.Version[len(metadata.Versioning.Versions.Version)-1]
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
