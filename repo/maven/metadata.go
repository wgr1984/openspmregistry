package maven

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
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
	Latest   string   `xml:"latest"`
	Release  string   `xml:"release"`
	Versions Versions `xml:"versions"`
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
