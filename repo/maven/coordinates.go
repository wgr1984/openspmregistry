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
// Format: {groupId-path}/{artifactId}/{version}/{artifactId}-{version}.{extension}
func buildMavenPath(groupId, artifactId, version, extension string) string {
	// Convert groupId dots to slashes
	groupIdPath := strings.ReplaceAll(groupId, ".", "/")
	
	// Build path: groupId-path/artifactId/version/artifactId-version.extension
	path := strings.Join([]string{
		groupIdPath,
		artifactId,
		version,
		artifactId + "-" + version + extension,
	}, "/")
	
	return path
}
