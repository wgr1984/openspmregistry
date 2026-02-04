package maven

import (
	"OpenSPMRegistry/config"
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_parseMetadata_ValidXML_ReturnsMetadata(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>com.example</groupId>
	<artifactId>test-package</artifactId>
	<versioning>
		<latest>2.0.0</latest>
		<release>2.0.0</release>
		<versions>
			<version>1.0.0</version>
			<version>1.1.0</version>
			<version>2.0.0</version>
		</versions>
	</versioning>
</metadata>`

	metadata, err := parseMetadata([]byte(xmlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metadata.GroupId != "com.example" {
		t.Errorf("expected GroupId 'com.example', got '%s'", metadata.GroupId)
	}
	if metadata.ArtifactId != "test-package" {
		t.Errorf("expected ArtifactId 'test-package', got '%s'", metadata.ArtifactId)
	}
	if metadata.Versioning.Latest != "2.0.0" {
		t.Errorf("expected Latest '2.0.0', got '%s'", metadata.Versioning.Latest)
	}
	if len(metadata.Versioning.Versions.Version) != 3 {
		t.Errorf("expected 3 versions, got %d", len(metadata.Versioning.Versions.Version))
	}
}

func Test_parseMetadata_InvalidXML_ReturnsError(t *testing.T) {
	invalidXML := `<invalid>xml</invalid>`
	_, err := parseMetadata([]byte(invalidXML))
	if err == nil {
		t.Errorf("expected error for invalid XML, got nil")
	}
}

func Test_getMetadataPath_ValidInput_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example.test"
	artifactId := "my-package"
	result := getMetadataPath(groupId, artifactId)
	expected := "com/example/test/my-package/maven-metadata.xml"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_getMetadataPath_SimpleGroupId_ReturnsCorrectPath(t *testing.T) {
	groupId := "test"
	artifactId := "package"
	result := getMetadataPath(groupId, artifactId)
	expected := "test/package/maven-metadata.xml"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_loadMetadata_ValidResponse_ReturnsMetadata(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8"?>
<metadata>
	<groupId>com.example</groupId>
	<artifactId>test-package</artifactId>
	<versioning>
		<versions>
			<version>1.0.0</version>
			<version>2.0.0</version>
		</versions>
	</versioning>
</metadata>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/com/example/test-package/maven-metadata.xml" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(xmlData))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	metadata, err := loadMetadata(c, context.Background(), "com.example", "test-package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metadata.GroupId != "com.example" {
		t.Errorf("expected GroupId 'com.example', got '%s'", metadata.GroupId)
	}
	if len(metadata.Versioning.Versions.Version) != 2 {
		t.Errorf("expected 2 versions, got %d", len(metadata.Versioning.Versions.Version))
	}
}

func Test_loadMetadata_NotFound_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = loadMetadata(c, context.Background(), "com.example", "test-package")
	if err == nil {
		t.Errorf("expected error for not found, got nil")
	}
}

func Test_loadMetadata_InvalidXML_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<invalid>xml</invalid>"))
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, err := newClient(cfg)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = loadMetadata(c, context.Background(), "com.example", "test-package")
	if err == nil {
		t.Errorf("expected error for invalid XML, got nil")
	}
}

// Test MavenMetadata XML marshaling/unmarshaling
func Test_MavenMetadata_XMLRoundTrip(t *testing.T) {
	original := &MavenMetadata{
		GroupId:    "com.example",
		ArtifactId: "test-package",
		Versioning: Versioning{
			Latest:  "2.0.0",
			Release: "2.0.0",
			Versions: Versions{
				Version: []string{"1.0.0", "2.0.0"},
			},
		},
	}

	data, err := xml.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	parsed, err := parseMetadata(data)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed.GroupId != original.GroupId {
		t.Errorf("GroupId mismatch: expected '%s', got '%s'", original.GroupId, parsed.GroupId)
	}
	if parsed.ArtifactId != original.ArtifactId {
		t.Errorf("ArtifactId mismatch: expected '%s', got '%s'", original.ArtifactId, parsed.ArtifactId)
	}
	if len(parsed.Versioning.Versions.Version) != len(original.Versioning.Versions.Version) {
		t.Errorf("Version count mismatch: expected %d, got %d", len(original.Versioning.Versions.Version), len(parsed.Versioning.Versions.Version))
	}
}
