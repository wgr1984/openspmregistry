package repo

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// GenerateCollection generates a package collection from the given packages.
// The collection name is prefixed with the provided hostname when available.
func GenerateCollection(ctx context.Context, r Repo, scope string, packages []models.ListElement, hostname string) (*models.PackageCollection, error) {
	// Group packages by scope/name without string splitting
	packagesByScope := make(map[string]map[string][]models.ListElement)
	for _, pkg := range packages {
		if _, ok := packagesByScope[pkg.Scope]; !ok {
			packagesByScope[pkg.Scope] = make(map[string][]models.ListElement)
		}
		packagesByScope[pkg.Scope][pkg.PackageName] = append(packagesByScope[pkg.Scope][pkg.PackageName], pkg)
	}

	// Build collection packages
	collectionPackages := make([]models.CollectionPackage, 0)
	for pkgScope, scopedPackages := range packagesByScope {
		for pkgName, versions := range scopedPackages {
			collPkg, err := buildCollectionPackage(ctx, r, pkgScope, pkgName, versions)
			if err != nil {
				slog.Warn("Error building collection package", "package", fmt.Sprintf("%s.%s", pkgScope, pkgName), "error", err)
				continue
			}

			// Skip packages with no valid versions
			if len(collPkg.Versions) == 0 {
				slog.Info("Skipping package with no valid versions", "package", fmt.Sprintf("%s.%s", pkgScope, pkgName))
				continue
			}

			collectionPackages = append(collectionPackages, *collPkg)
		}
	}

	// sort collection packages by (lowercase) URL in ascending order
	slices.SortFunc(collectionPackages, func(a models.CollectionPackage, b models.CollectionPackage) int {
		return strings.Compare(strings.ToLower(a.URL), strings.ToLower(b.URL))
	})

	// Build collection metadata
	collectionName := "All Packages"
	collectionOverview := "All packages in registry"
	if scope != "" {
		collectionName = fmt.Sprintf("%s Packages", scope)
		collectionOverview = fmt.Sprintf("Package collection for %s scope", scope)
	}

	if trimmedHostname := strings.TrimSpace(hostname); trimmedHostname != "" {
		collectionName = fmt.Sprintf("%s: %s", trimmedHostname, collectionName)
	}

	collection := &models.PackageCollection{
		Name:          collectionName,
		Overview:      collectionOverview,
		Keywords:      []string{},
		Packages:      collectionPackages,
		FormatVersion: "1.0",
		Revision:      1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		GeneratedBy: models.GeneratedBy{
			Name: "OpenSPMRegistry",
		},
	}

	return collection, nil
}

// buildCollectionPackage builds a CollectionPackage from package versions
func buildCollectionPackage(ctx context.Context, r Repo, scope string, name string, versionElements []models.ListElement) (*models.CollectionPackage, error) {
	// Prefer newest versions first so we pick metadata from the latest included version
	sortVersionsDesc(versionElements)

	// Build package versions
	var packageVersions []models.PackageVersion
	var metadataVersion string
	for _, versionElement := range versionElements {
		pkgVersion, err := buildPackageVersion(ctx, r, scope, name, versionElement.Version)
		if err != nil {
			slog.Warn("Skipping version without Package.json", "package", fmt.Sprintf("%s.%s", scope, name), "version", versionElement.Version, "error", err)
			continue
		}
		packageVersions = append(packageVersions, *pkgVersion)

		if metadataVersion == "" {
			metadataVersion = versionElement.Version
		}
	}

	// Get metadata for summary, license, and repository URL
	var summary string
	var license *models.License
	var readmeURL string

	if metadataVersion != "" {
		// Use metadata from the first version that is actually included
		metadata, err := r.LoadMetadata(ctx, scope, name, metadataVersion)
		if err == nil {
			if desc, ok := metadata["description"].(string); ok {
				summary = desc
			}
			if licenseURLStr, ok := metadata["licenseURL"].(string); ok {
				license = &models.License{URL: licenseURLStr}
			}
			if readmeURLStr, ok := metadata["readmeURL"].(string); ok {
				readmeURL = readmeURLStr
			}
		}
	}

	collectionPackage := &models.CollectionPackage{
		URL:       fmt.Sprintf("%s.%s", scope, name),
		Summary:   summary,
		Keywords:  []string{},
		Versions:  packageVersions,
		ReadmeURL: readmeURL,
		License:   license,
	}

	return collectionPackage, nil
}

// sortVersionsDesc sorts list elements by semantic-ish version in descending order (best effort)
func sortVersionsDesc(versionElements []models.ListElement) {
	slices.SortFunc(versionElements, func(a models.ListElement, b models.ListElement) int {
		v1, err := models.ParseVersion(strings.TrimSpace(a.Version))
		if err != nil {
			return 0
		}
		v2, err := models.ParseVersion(strings.TrimSpace(b.Version))
		if err != nil {
			return 0
		}
		// Descending: newer (larger) first
		return v2.Compare(v1)
	})
}

// buildPackageVersion builds a PackageVersion from a specific version
func buildPackageVersion(ctx context.Context, r Repo, scope string, name string, version string) (*models.PackageVersion, error) {
	// Load Package.json
	packageJson, err := r.LoadPackageJson(ctx, scope, name, version)
	if err != nil {
		return nil, err
	}

	// Get tools version from Package.swift
	manifestElement := models.NewUploadElement(scope, name, version, mimetypes.TextXSwift, models.Manifest)
	toolsVersionStr, err := r.GetSwiftToolVersion(ctx, manifestElement)
	if err != nil {
		slog.Warn("Could not get tools version", "package", fmt.Sprintf("%s.%s@%s", scope, name, version), "error", err)
		toolsVersionStr = "5.0" // default
	}

	// Strip patch version from tools version (e.g., "5.10.0" -> "5.10")
	toolsVersion := stripPatchVersion(strings.TrimSpace(toolsVersionStr))

	// Convert Package.json to manifest
	manifest := convertPackageJsonToManifest(packageJson, toolsVersion)

	// Build manifests map
	manifests := map[string]models.PackageManifest{
		toolsVersion: manifest,
	}

	// Get metadata for author info
	metadata, _ := r.LoadMetadata(ctx, scope, name, version)
	var author *models.Author
	var license *models.License

	if metadata != nil {
		author = extractAuthor(metadata)
		if licenseURLStr, ok := metadata["licenseURL"].(string); ok {
			licenseName := "License"
			if licName, ok := metadata["licenseName"].(string); ok {
				licenseName = licName
			}
			license = &models.License{Name: licenseName, URL: licenseURLStr}
		}
	}

	// Get publish date
	sourceArchiveElement := models.NewUploadElement(scope, name, version, "application/zip", models.SourceArchive)
	publishDate, err := r.PublishDate(ctx, sourceArchiveElement)
	if err != nil {
		publishDate = time.Now()
	}

	packageVersion := &models.PackageVersion{
		Version:             version,
		Manifests:           manifests,
		DefaultToolsVersion: toolsVersion,
		Author:              author,
		License:             license,
		CreatedAt:           publishDate.UTC().Format(time.RFC3339),
	}

	return packageVersion, nil
}

// convertPackageJsonToManifest converts Package.json (swift package dump-package output) to SE-0291 manifest format
func convertPackageJsonToManifest(packageJson map[string]any, toolsVersion string) models.PackageManifest {
	manifest := models.PackageManifest{
		ToolsVersion: toolsVersion,
		Targets:      []models.Target{},
		Products:     []models.Product{},
	}

	// Extract package name
	if name, ok := packageJson["name"].(string); ok {
		manifest.PackageName = name
	}

	// Extract targets (only regular targets, skip test targets)
	if targets, ok := packageJson["targets"].([]any); ok {
		for _, t := range targets {
			if targetMap, ok := t.(map[string]any); ok {
				// Skip test targets
				if targetType, ok := targetMap["type"].(string); ok && targetType == "test" {
					continue
				}

				if targetName, ok := targetMap["name"].(string); ok {
					target := models.Target{
						Name: targetName,
					}
					// Only set moduleName if it's different from name (optional in spec)
					if moduleName, ok := targetMap["moduleName"].(string); ok && moduleName != targetName {
						target.ModuleName = moduleName
					}
					manifest.Targets = append(manifest.Targets, target)
				}
			}
		}
	}

	// Extract products
	if products, ok := packageJson["products"].([]any); ok {
		for _, p := range products {
			if productMap, ok := p.(map[string]any); ok {
				product := models.Product{
					Type: make(map[string][]string),
				}

				if productName, ok := productMap["name"].(string); ok {
					product.Name = productName
				}

				// Extract targets
				if targetsArr, ok := productMap["targets"].([]any); ok {
					for _, t := range targetsArr {
						if targetStr, ok := t.(string); ok {
							product.Targets = append(product.Targets, targetStr)
						}
					}
				}

				// Extract product type (library: [automatic], library: [dynamic], executable, etc.)
				if productType, ok := productMap["type"].(map[string]any); ok {
					for typeKey, typeValue := range productType {
						if typeValue == nil {
							// Handle null values (common for executable: null)
							product.Type[typeKey] = []string{}
						} else if typeArr, ok := typeValue.([]any); ok {
							var typeStrs []string
							for _, tv := range typeArr {
								if tvStr, ok := tv.(string); ok {
									typeStrs = append(typeStrs, tvStr)
								}
							}
							product.Type[typeKey] = typeStrs
						}
					}
				}

				manifest.Products = append(manifest.Products, product)
			}
		}
	}

	// Extract platforms
	if platforms, ok := packageJson["platforms"].([]any); ok {
		for _, p := range platforms {
			if platformMap, ok := p.(map[string]any); ok {
				platformName, hasName := platformMap["platformName"].(string)
				platformVersion, hasVersion := platformMap["version"].(string)

				if hasName && hasVersion {
					manifest.MinimumPlatformVersions = append(manifest.MinimumPlatformVersions, models.MinimumPlatformVersion{
						Name:    platformName, // Already lowercase in Package.json
						Version: platformVersion,
					})
				}
			}
		}
	}

	return manifest
}

// extractAuthor extracts author information from metadata
func extractAuthor(metadata map[string]any) *models.Author {
	if authorData, ok := metadata["author"].(map[string]any); ok {
		author := &models.Author{}

		if name, ok := authorData["name"].(string); ok {
			author.Name = name
			return author
		}
	}

	return nil
}

// stripPatchVersion strips the patch version from a tools version string
// e.g., "5.10.0" -> "5.10", "6.0.0" -> "6.0"
func stripPatchVersion(version string) string {
	version = strings.TrimSpace(version)
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return fmt.Sprintf("%s.%s", parts[0], parts[1])
	}
	return version
}
