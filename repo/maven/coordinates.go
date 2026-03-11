package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"strings"
)

// buildGroupId converts SPM scope to Maven groupId
func buildGroupId(scope string, cfg config.MavenConfig) string {
	if cfg.GroupIdPrefix == "" {
		return scope
	}
	return cfg.GroupIdPrefix + "." + scope
}

// groupIdToScope converts Maven groupId to SPM scope (inverse of buildGroupId).
// If GroupIdPrefix is set and groupId has that prefix, returns groupId without the prefix; otherwise returns groupId.
// Example:
// - groupId: "com-example.testScope"
// - cfg.GroupIdPrefix: "com-example"
// - Returns: "testScope"
func groupIdToScope(groupId string, cfg config.MavenConfig) string {
	if cfg.GroupIdPrefix == "" {
		return groupId
	}
	prefix := cfg.GroupIdPrefix + "."
	if len(groupId) >= len(prefix) && groupId[:len(prefix)] == prefix {
		return groupId[len(prefix):]
	}
	return groupId
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
//   - groupId: Maven group ID (e.g., "com.example")
//   - artifactId: Maven artifact ID (e.g., "my-package")
//   - version: Artifact version (e.g., "1.0.0")
//   - classifier: Optional classifier (e.g., "sources", "javadoc", "tests", "test-jar", or empty string for main artifact)
//     Multiple dashes within classifier are valid (e.g., "package-swift-5.7.0", "sources-javadoc")
//   - extension: File extension (e.g., ".jar", ".pom", ".war", ".zip")
//
// Returns:
//   - Maven repository path (e.g., "com/example/my-package/1.0.0/my-package-1.0.0.jar")
//     With classifier: "com/example/my-package/1.0.0/my-package-1.0.0-sources.jar"
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

// mavenClassifierFromFilename converts an SPM filename base to a Maven classifier.
// Lowercase, @ replaced with -, and "swift-" prefix added when missing (e.g. Package@5.7.0 → package-swift-5.7.0).
func mavenClassifierFromFilename(filename string) string {
	if filename == "" {
		return ""
	}
	s := strings.ToLower(filename)
	s = strings.ReplaceAll(s, "@", "-")
	parts := strings.Split(s, "-")
	if len(parts) >= 2 && parts[1] != "swift" {
		parts = append(parts[:1], append([]string{"swift"}, parts[1:]...)...)
		s = strings.Join(parts, "-")
	}
	return s
}

// pathPartsForElement returns the Maven classifier and extension for an element.
// Sidecars (metadata, manifests) get a classifier; main artifact does not.
func pathPartsForElement(element *models.UploadElement) (classifier, ext string) {
	fn := element.FilenameWithoutExtension()
	isSidecar := fn == "metadata" || strings.HasPrefix(strings.ToLower(fn), "package")
	if isSidecar {
		classifier = mavenClassifierFromFilename(fn)
	}
	ext = element.Extension()
	if ext != "" && ext[0] != '.' {
		ext = "." + ext
	}
	return classifier, ext
}

// buildMavenPathForElement builds the Maven repository path for an SPM element.
func buildMavenPathForElement(element *models.UploadElement, cfg config.MavenConfig) string {
	groupId := buildGroupId(element.Scope, cfg)
	artifactId := buildArtifactId(element.Name)
	version := buildVersion(element.Version)
	classifier, ext := pathPartsForElement(element)
	return buildMavenPath(groupId, artifactId, version, classifier, ext)
}
