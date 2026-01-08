package models

import (
	"encoding/json"
	"testing"
)

func TestPackageCollectionJSONMarshaling(t *testing.T) {
	collection := PackageCollection{
		Name:          "Test Collection",
		Overview:      "A test collection",
		Keywords:      []string{"test", "example"},
		FormatVersion: "1.0",
		GeneratedAt:   "2024-01-06T12:00:00Z",
		GeneratedBy: GeneratedBy{
			Name: "OpenSPMRegistry",
		},
		Packages: []CollectionPackage{
			{
				URL:     "ext.Alamofire",
				Summary: "Elegant HTTP Networking in Swift",
				Versions: []PackageVersion{
					{
						Version:             "5.10.0",
						DefaultToolsVersion: "5.10",
						Manifests: map[string]PackageManifest{
							"5.10": {
								ToolsVersion: "5.10",
								PackageName:  "Alamofire",
								Targets: []Target{
									{Name: "Alamofire", ModuleName: "Alamofire"},
								},
								Products: []Product{
									{
										Name:    "Alamofire",
										Type:    map[string][]string{"library": {"automatic"}},
										Targets: []string{"Alamofire"},
									},
								},
								MinimumPlatformVersions: []MinimumPlatformVersion{
									{Name: "macos", Version: "10.13"},
									{Name: "ios", Version: "12.0"},
								},
							},
						},
						Author: &Author{
							Name:  "Alamofire Software Foundation",
							Email: "info@alamofire.org",
						},
						CreatedAt: "2024-01-06T04:00:00Z",
					},
				},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(collection)
	if err != nil {
		t.Fatalf("Failed to marshal collection: %v", err)
	}

	// Unmarshal back
	var unmarshaled PackageCollection
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal collection: %v", err)
	}

	// Verify key fields
	if unmarshaled.Name != "Test Collection" {
		t.Errorf("Expected name 'Test Collection', got '%s'", unmarshaled.Name)
	}

	if unmarshaled.FormatVersion != "1.0" {
		t.Errorf("Expected formatVersion '1.0', got '%s'", unmarshaled.FormatVersion)
	}

	if len(unmarshaled.Packages) != 1 {
		t.Errorf("Expected 1 package, got %d", len(unmarshaled.Packages))
	}

	if len(unmarshaled.Packages) > 0 {
		pkg := unmarshaled.Packages[0]
		if pkg.URL != "ext.Alamofire" {
			t.Errorf("Expected package URL 'ext.Alamofire', got '%s'", pkg.URL)
		}

		if len(pkg.Versions) != 1 {
			t.Errorf("Expected 1 version, got %d", len(pkg.Versions))
		}

		if len(pkg.Versions) > 0 {
			version := pkg.Versions[0]
			if version.Version != "5.10.0" {
				t.Errorf("Expected version '5.10.0', got '%s'", version.Version)
			}

			if version.DefaultToolsVersion != "5.10" {
				t.Errorf("Expected defaultToolsVersion '5.10', got '%s'", version.DefaultToolsVersion)
			}
		}
	}
}

func TestPackageCollectionJSONFieldNames(t *testing.T) {
	collection := PackageCollection{
		Name:          "Test",
		FormatVersion: "1.0",
		GeneratedAt:   "2024-01-06T12:00:00Z",
	}

	data, err := json.Marshal(collection)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)

	// Verify camelCase field names
	expectedFields := []string{
		"\"name\"",
		"\"formatVersion\"",
		"\"generatedAt\"",
		"\"packages\"",
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("Expected JSON to contain field %s", field)
		}
	}
}

func TestMinimumPlatformVersion(t *testing.T) {
	platforms := []MinimumPlatformVersion{
		{Name: "macos", Version: "10.13"},
		{Name: "ios", Version: "12.0"},
		{Name: "tvos", Version: "12.0"},
		{Name: "watchos", Version: "4.0"},
	}

	data, err := json.Marshal(platforms)
	if err != nil {
		t.Fatalf("Failed to marshal platforms: %v", err)
	}

	var unmarshaled []MinimumPlatformVersion
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal platforms: %v", err)
	}

	if len(unmarshaled) != 4 {
		t.Errorf("Expected 4 platforms, got %d", len(unmarshaled))
	}

	// Verify platform names are lowercase
	for _, platform := range unmarshaled {
		if platform.Name != "macos" && platform.Name != "ios" && platform.Name != "tvos" && platform.Name != "watchos" {
			t.Errorf("Unexpected platform name: %s", platform.Name)
		}
	}
}

func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || len(str) > len(substr) && (string(str[0:len(substr)]) == substr || contains(str[1:], substr)))
}

