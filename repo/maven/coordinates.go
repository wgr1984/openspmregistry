package maven

import (
	"OpenSPMRegistry/config"
	"strings"
)

// buildGroupId converts SPM scope to Maven groupId
func buildGroupId(scope string, cfg config.MavenConfig) string {
	if cfg.GroupIdPrefix == "" {
		return scope
	}
	return cfg.GroupIdPrefix + "." + scope
}

// buildArtifactId converts SPM name to Maven artifactId (direct mapping)
func buildArtifactId(name string) string {
	return name
}

// buildVersion converts SPM version to Maven version (direct mapping)
func buildVersion(version string) string {
	return version
}

// buildMavenPath constructs the Maven repository path for an artifact
// Format: {groupId-path}/{artifactId}/{version}/{artifactId}-{version}[-{classifier}].{extension}
//
// Where:
// - {groupId-path}: groupId with dots replaced by slashes (e.g., "com.example" → "com/example")
// - {artifactId}: artifact identifier
// - {version}: artifact version
// - [-{classifier}]: optional classifier, prefixed with hyphen if present (e.g., "-sources", "-javadoc")
// - {extension}: file extension (e.g., ".jar", ".pom", ".zip")
//
// Parameters:
// - groupId: Maven group ID (e.g., "com.example")
// - artifactId: Maven artifact ID (e.g., "my-package")
// - version: Artifact version (e.g., "1.0.0")
// - classifier: Optional classifier (e.g., "sources", "javadoc", "tests", "test-jar", or empty string for main artifact)
//   Multiple dashes within classifier are valid (e.g., "package-swift-5.7.0", "sources-javadoc")
// - extension: File extension (e.g., ".jar", ".pom", ".war", ".zip")
//
// Returns:
// - Maven repository path (e.g., "com/example/my-package/1.0.0/my-package-1.0.0.jar")
//   With classifier: "com/example/my-package/1.0.0/my-package-1.0.0-sources.jar"
func buildMavenPath(groupId, artifactId, version, classifier, extension string) string {
	// Convert groupId dots to slashes
	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	
	// Build filename: artifactId-version[-classifier].extension
	filename := artifactId + "-" + version
	if classifier != "" {
		filename += "-" + classifier
	}
	filename += extension
	
	// Build path: groupId-path/artifactId/version/filename
	path := strings.Join([]string{
		groupIdPath,
		artifactId,
		version,
		filename,
	}, "/")
	
	return path
}
