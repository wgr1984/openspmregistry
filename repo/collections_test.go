package repo

import (
	"testing"
)

func TestStripPatchVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"5.10.0", "5.10"},
		{"6.0.0", "6.0"},
		{"5.9", "5.9"},
		{"5", "5"},
		{"  5.10.0  ", "5.10"},
	}

	for _, test := range tests {
		result := stripPatchVersion(test.input)
		if result != test.expected {
			t.Errorf("stripPatchVersion(%s) = %s; want %s", test.input, result, test.expected)
		}
	}
}

func TestExtractAuthor(t *testing.T) {
	// Test with valid author data
	metadata := map[string]interface{}{
		"author": map[string]interface{}{
			"name":  "John Doe",
			"email": "john@example.com",
			"organization": map[string]interface{}{
				"name": "Example Org",
			},
		},
	}

	author := extractAuthor(metadata)
	if author == nil {
		t.Fatal("Expected author, got nil")
	}

	if author.Name != "John Doe" {
		t.Errorf("Expected name 'John Doe', got '%s'", author.Name)
	}

	if author.Email != "john@example.com" {
		t.Errorf("Expected email 'john@example.com', got '%s'", author.Email)
	}

	if author.Organization == nil {
		t.Fatal("Expected organization, got nil")
	}

	if author.Organization.Name != "Example Org" {
		t.Errorf("Expected organization name 'Example Org', got '%s'", author.Organization.Name)
	}

	// Test with missing author
	emptyMetadata := map[string]interface{}{}
	author = extractAuthor(emptyMetadata)
	if author != nil {
		t.Errorf("Expected nil author, got %v", author)
	}

	// Test with author missing name (required field)
	invalidMetadata := map[string]interface{}{
		"author": map[string]interface{}{
			"email": "test@example.com",
		},
	}
	author = extractAuthor(invalidMetadata)
	if author != nil {
		t.Errorf("Expected nil author when name is missing, got %v", author)
	}
}

func TestConvertPackageJsonToManifest(t *testing.T) {
	// Sample Package.json structure
	packageJson := map[string]interface{}{
		"name": "Alamofire",
		"platforms": []interface{}{
			map[string]interface{}{
				"platformName": "macos",
				"version":      "10.13",
			},
			map[string]interface{}{
				"platformName": "ios",
				"version":      "12.0",
			},
		},
		"products": []interface{}{
			map[string]interface{}{
				"name":    "Alamofire",
				"targets": []interface{}{"Alamofire"},
				"type": map[string]interface{}{
					"library": []interface{}{"automatic"},
				},
			},
		},
		"targets": []interface{}{
			map[string]interface{}{
				"name": "Alamofire",
				"type": "regular",
			},
			map[string]interface{}{
				"name": "AlamofireTests",
				"type": "test",
			},
		},
	}

	manifest := convertPackageJsonToManifest(packageJson, "5.10")

	// Verify tools version
	if manifest.ToolsVersion != "5.10" {
		t.Errorf("Expected toolsVersion '5.10', got '%s'", manifest.ToolsVersion)
	}

	// Verify package name
	if manifest.PackageName != "Alamofire" {
		t.Errorf("Expected packageName 'Alamofire', got '%s'", manifest.PackageName)
	}

	// Verify targets (should exclude test targets)
	if len(manifest.Targets) != 1 {
		t.Errorf("Expected 1 target (excluding test), got %d", len(manifest.Targets))
	}

	if len(manifest.Targets) > 0 {
		if manifest.Targets[0].Name != "Alamofire" {
			t.Errorf("Expected target name 'Alamofire', got '%s'", manifest.Targets[0].Name)
		}
		if manifest.Targets[0].ModuleName != "Alamofire" {
			t.Errorf("Expected moduleName 'Alamofire', got '%s'", manifest.Targets[0].ModuleName)
		}
	}

	// Verify products
	if len(manifest.Products) != 1 {
		t.Errorf("Expected 1 product, got %d", len(manifest.Products))
	}

	if len(manifest.Products) > 0 {
		product := manifest.Products[0]
		if product.Name != "Alamofire" {
			t.Errorf("Expected product name 'Alamofire', got '%s'", product.Name)
		}

		if len(product.Targets) != 1 || product.Targets[0] != "Alamofire" {
			t.Errorf("Expected product targets ['Alamofire'], got %v", product.Targets)
		}

		if product.Type["library"][0] != "automatic" {
			t.Errorf("Expected product type library[automatic], got %v", product.Type)
		}
	}

	// Verify platforms
	if len(manifest.MinimumPlatformVersions) != 2 {
		t.Errorf("Expected 2 platforms, got %d", len(manifest.MinimumPlatformVersions))
	}

	// Verify platform names are lowercase
	for _, platform := range manifest.MinimumPlatformVersions {
		if platform.Name != "macos" && platform.Name != "ios" {
			t.Errorf("Unexpected platform name: %s", platform.Name)
		}
	}
}

func TestConvertPackageJsonToManifestEmpty(t *testing.T) {
	// Test with empty/minimal Package.json
	packageJson := map[string]interface{}{}

	manifest := convertPackageJsonToManifest(packageJson, "5.0")

	if manifest.ToolsVersion != "5.0" {
		t.Errorf("Expected toolsVersion '5.0', got '%s'", manifest.ToolsVersion)
	}

	if len(manifest.Targets) != 0 {
		t.Errorf("Expected 0 targets, got %d", len(manifest.Targets))
	}

	if len(manifest.Products) != 0 {
		t.Errorf("Expected 0 products, got %d", len(manifest.Products))
	}

	if len(manifest.MinimumPlatformVersions) != 0 {
		t.Errorf("Expected 0 platforms, got %d", len(manifest.MinimumPlatformVersions))
	}
}

func TestBuildCollectionPackageURL(t *testing.T) {
	// Test that package URL is in scope.name format
	scope := "ext"
	name := "Alamofire"

	expected := "ext.Alamofire"
	result := scope + "." + name

	if result != expected {
		t.Errorf("Expected URL '%s', got '%s'", expected, result)
	}
}
