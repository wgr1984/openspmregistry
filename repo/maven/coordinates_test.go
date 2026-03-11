package maven

import (
	"OpenSPMRegistry/config"
	"strings"
	"testing"
)

func Test_buildGroupId_NoPrefix_ReturnsScope(t *testing.T) {
	cfg := config.MavenConfig{}
	result := buildGroupId("testScope", cfg)
	if result != "testScope" {
		t.Errorf("expected 'testScope', got '%s'", result)
	}
}

// groupId = prefix + "." + scope; single-segment prefix so groupId has exactly one dot.
func Test_buildGroupId_WithPrefix_ReturnsPrefixedScope(t *testing.T) {
	cfg := config.MavenConfig{GroupIdPrefix: "com"}
	result := buildGroupId("testScope", cfg)
	expected := "com.testScope"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_groupIdToScope_NoPrefix_ReturnsGroupId(t *testing.T) {
	cfg := config.MavenConfig{}
	result := groupIdToScope("testScope", cfg)
	if result != "testScope" {
		t.Errorf("expected 'testScope', got '%s'", result)
	}
}

// Per SPM Registry, the only dot is between scope and package name (scope.packageName).
// So scope is a single segment — no dots. groupId is prefix + "." + scope when prefix is set.
func Test_groupIdToScope_WithPrefix_ReturnsScopeWithoutPrefix(t *testing.T) {
	cfg := config.MavenConfig{GroupIdPrefix: "com"} // single-segment prefix so groupId has exactly one dot
	result := groupIdToScope("com.testScope", cfg)
	if result != "testScope" {
		t.Errorf("expected 'testScope', got '%s'", result)
	}
	if strings.Contains(result, ".") {
		t.Errorf("scope must be a single segment (only one dot exists between scope and package name); got %q", result)
	}
}

func Test_groupIdToScope_WithPrefix_NoMatch_ReturnsGroupId(t *testing.T) {
	cfg := config.MavenConfig{GroupIdPrefix: "com"} // same single-segment prefix as other prefix tests
	// groupId does not have prefix "com." so return as-is; single segment so valid scope shape
	result := groupIdToScope("otherscope", cfg)
	if result != "otherscope" {
		t.Errorf("expected 'otherscope', got '%s'", result)
	}
}

func Test_buildArtifactId_ReturnsName(t *testing.T) {
	result := buildArtifactId("test-package")
	if result != "test-package" {
		t.Errorf("expected 'test-package', got '%s'", result)
	}
}

func Test_buildVersion_ReturnsVersion(t *testing.T) {
	result := buildVersion("1.2.3")
	if result != "1.2.3" {
		t.Errorf("expected '1.2.3', got '%s'", result)
	}
}

func Test_buildMavenPath_ValidInput_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example.test"
	artifactId := "my-package"
	version := "1.0.0"
	classifier := ""
	extension := ".zip"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/test/my-package/1.0.0/my-package-1.0.0.zip"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_buildMavenPath_SimpleGroupId_ReturnsCorrectPath(t *testing.T) {
	groupId := "test"
	artifactId := "package"
	version := "1.0.0"
	classifier := ""
	extension := ".jar"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "test/package/1.0.0/package-1.0.0.jar"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func Test_buildMavenPath_WithClassifier_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "artifact"
	version := "2.0.0"
	classifier := "metadata"
	extension := ".json"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/artifact/2.0.0/artifact-2.0.0-metadata.json"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_PomFile_ReturnsCorrectPath tests POM file path construction
// According to Maven spec: POM files have extension "pom" and no classifier
func Test_buildMavenPath_PomFile_ReturnsCorrectPath(t *testing.T) {
	groupId := "org.apache.maven"
	artifactId := "maven-core"
	version := "3.9.0"
	classifier := ""
	extension := ".pom"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "org/apache/maven/maven-core/3.9.0/maven-core-3.9.0.pom"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_WarFile_ReturnsCorrectPath tests WAR file path construction
// According to Maven spec: WAR files have extension "war" and no classifier
func Test_buildMavenPath_WarFile_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "webapp"
	version := "1.0.0"
	classifier := ""
	extension := ".war"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/webapp/1.0.0/webapp-1.0.0.war"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_DefaultJar_ReturnsCorrectPath tests default JAR file path
// According to Maven spec: JAR is the default extension
func Test_buildMavenPath_DefaultJar_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "library"
	version := "2.5.0"
	classifier := ""
	extension := ".jar"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/library/2.5.0/library-2.5.0.jar"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_SourcesJar_ReturnsCorrectPath tests sources JAR
// According to Maven spec: artifact-version-sources.jar
// where "sources" is the classifier and "jar" is the extension
func Test_buildMavenPath_SourcesJar_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "library"
	version := "2.5.0"
	classifier := "sources"
	extension := ".jar"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/library/2.5.0/library-2.5.0-sources.jar"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_JavadocJar_ReturnsCorrectPath tests javadoc JAR
// According to Maven spec: artifact-version-javadoc.jar
// where "javadoc" is the classifier and "jar" is the extension
func Test_buildMavenPath_JavadocJar_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "library"
	version := "2.5.0"
	classifier := "javadoc"
	extension := ".jar"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/library/2.5.0/library-2.5.0-javadoc.jar"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test_buildMavenPath_TestJar_ReturnsCorrectPath tests test JAR
// According to Maven spec: artifact-version-tests.jar
// where "tests" is the classifier and "jar" is the extension
func Test_buildMavenPath_TestJar_ReturnsCorrectPath(t *testing.T) {
	groupId := "com.example"
	artifactId := "library"
	version := "2.5.0"
	classifier := "tests"
	extension := ".jar"

	result := buildMavenPath(groupId, artifactId, version, classifier, extension)
	expected := "com/example/library/2.5.0/library-2.5.0-tests.jar"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}
