package models

// PackageCollection represents a complete SE-0291 package collection.
// https://github.com/apple/swift-package-collection-generator/blob/main/PackageCollectionFormats/v1.md
type PackageCollection struct {
	Name          string              `json:"name"`
	Overview      string              `json:"overview,omitempty"`
	Keywords      []string            `json:"keywords,omitempty"`
	Packages      []CollectionPackage `json:"packages"`
	FormatVersion string              `json:"formatVersion"`
	Revision      int                 `json:"revision,omitempty"`
	GeneratedAt   string              `json:"generatedAt"`
	GeneratedBy   GeneratedBy         `json:"generatedBy,omitempty"`
}

// GeneratedBy describes who generated the collection
type GeneratedBy struct {
	Name string `json:"name,omitempty"`
}

// CollectionPackage represents a package within a collection
type CollectionPackage struct {
	URL       string           `json:"url"`
	Summary   string           `json:"summary,omitempty"`
	Keywords  []string         `json:"keywords,omitempty"`
	Versions  []PackageVersion `json:"versions"`
	ReadmeURL string           `json:"readmeURL,omitempty"`
	License   *License         `json:"license,omitempty"`
}

// PackageVersion represents a specific version of a package
type PackageVersion struct {
	Version               string                     `json:"version"`
	Summary               string                     `json:"summary,omitempty"`
	Manifests             map[string]PackageManifest `json:"manifests"`
	DefaultToolsVersion   string                     `json:"defaultToolsVersion"`
	VerifiedCompatibility []VerifiedCompatibility    `json:"verifiedCompatibility,omitempty"`
	License               *License                   `json:"license,omitempty"`
	Author                *Author                    `json:"author,omitempty"`
	Signer                *Signer                    `json:"signer,omitempty"`
	CreatedAt             string                     `json:"createdAt,omitempty"`
}

// PackageManifest represents the manifest details for a specific tools version
type PackageManifest struct {
	ToolsVersion            string                   `json:"toolsVersion"`
	PackageName             string                   `json:"packageName"`
	Targets                 []Target                 `json:"targets"`
	Products                []Product                `json:"products"`
	MinimumPlatformVersions []MinimumPlatformVersion `json:"minimumPlatformVersions,omitempty"`
}

// Target represents a target in the manifest
type Target struct {
	Name       string `json:"name"`
	ModuleName string `json:"moduleName,omitempty"`
}

// Product represents a product in the manifest
type Product struct {
	Name    string              `json:"name"`
	Type    map[string][]string `json:"type,omitempty"`
	Targets []string            `json:"targets"`
}

// MinimumPlatformVersion represents platform requirements
type MinimumPlatformVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// VerifiedCompatibility represents verified platform/Swift compatibility
type VerifiedCompatibility struct {
	Platform     Platform `json:"platform"`
	SwiftVersion string   `json:"swiftVersion"`
}

// Platform represents a platform in verified compatibility
type Platform struct {
	Name string `json:"name"`
}

// Author represents author information
type Author struct {
	Name string `json:"name"`
}

// License represents license information
type License struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
}

// Signer represents package signing information
type Signer struct {
	CommonName   string `json:"commonName,omitempty"`
	Organization string `json:"organization,omitempty"`
}
