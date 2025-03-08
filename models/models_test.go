package models

import (
	"testing"
)

func Test_Compare_VersionsAreEqual_ReturnsZero(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0}
	v2 := Version{Major: 1, Minor: 0, Patch: 0}

	result := v1.Compare(&v2)
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

func Test_Compare_Version1IsGreater_ReturnsPositive(t *testing.T) {
	v1 := Version{Major: 2, Minor: 0, Patch: 0}
	v2 := Version{Major: 1, Minor: 0, Patch: 0}

	result := v1.Compare(&v2)
	if result <= 0 {
		t.Errorf("expected positive, got %d", result)
	}
}

func Test_Compare_Version1IsLesser_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0}
	v2 := Version{Major: 2, Minor: 0, Patch: 0}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version1IsLesser_ReturnsNegative_2(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0}
	v2 := Version{Major: 1, Minor: 0, Patch: 2}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version1IsBiggerBySuffix_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0}
	v2 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "alpha"}

	result := v1.Compare(&v2)
	if result <= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version1IsLesserBySuffix_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "alpha"}
	v2 := Version{Major: 1, Minor: 0, Patch: 0}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version2AlphaIsLesserThanBeta_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "alpha"}
	v2 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "beta"}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version2SnapshotIsLesserThanRelease_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "snapshot"}
	v2 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "release"}

	result := v1.Compare(&v2)
	if result <= 0 {
		t.Errorf("expected positive, got %d", result)
	}
}

func Test_Compare_Version1SnapshotIsLesserThanRelease_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "release"}
	v2 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "snapshot"}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_Compare_Version1SnapshotIsLesserThanNoSuffix_ReturnsNegative(t *testing.T) {
	v1 := Version{Major: 1, Minor: 0, Patch: 0, Suffix: "snapshot"}
	v2 := Version{Major: 1, Minor: 0, Patch: 0}

	result := v1.Compare(&v2)
	if result >= 0 {
		t.Errorf("expected negative, got %d", result)
	}
}

func Test_ParseVersion_ValidVersionString_ReturnsVersion(t *testing.T) {
	versionStr := "1.2.3"
	expected := &Version{Major: 1, Minor: 2, Patch: 3}

	result, err := ParseVersion(versionStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result != *expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func Test_ParseVersion_InvalidVersionString_ReturnsError(t *testing.T) {
	versionStr := "invalid"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_FileName_NoOverwrite_ReturnsDefaultName(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "application/zip"}
	expected := "scope.name-1.0.0.zip"

	result := element.FileName()
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_FileName_WithFilenameOverwrite_ReturnsOverwrittenName(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "application/zip"}
	element.SetFilenameOverwrite("custom")
	expected := "custom.zip"

	result := element.FileName()
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_FileName_WithExtOverwrite_ReturnsOverwrittenExt(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "application/zip"}
	element.SetExtOverwrite(".tar.gz")
	expected := "scope.name-1.0.0.tar.gz"

	result := element.FileName()
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_SortVersions_ValidVersions_ReturnsSorted(t *testing.T) {
	versions := []string{"1.0.0", "2.0.0", "1.1.0"}
	expected := []string{"2.0.0", "1.1.0", "1.0.0"}

	result := SortVersions(versions)
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], v)
		}
	}
}

func Test_MarshalJSON_NilListRelease_ReturnsNull(t *testing.T) {
	var listRelease *ListRelease

	data, err := listRelease.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected null, got %s", string(data))
	}
}

func Test_MarshalJSON_EmptyReleases_ReturnsEmptyObject(t *testing.T) {
	listRelease := &ListRelease{Releases: map[string]Release{}}

	data, err := listRelease.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"releases":{}}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func Test_MarshalJSON_NonEmptyReleases_ReturnsSortedReleases(t *testing.T) {
	releases := map[string]Release{
		"1.0.0": {Url: "url1"},
		"2.0.0": {Url: "url2"},
		"1.1.0": {Url: "url3"},
	}
	listRelease := &ListRelease{Releases: releases}

	data, err := listRelease.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"releases":{"2.0.0":{"url":"url2"},"1.1.0":{"url":"url3"},"1.0.0":{"url":"url1"}}}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func Test_sortReleases_ValidReleases_ReturnsSortedKeysAndReleases(t *testing.T) {
	releases := map[string]Release{
		"1.0.0": {Url: "url1"},
		"2.0.0": {Url: "url2"},
		"1.1.0": {Url: "url3"},
	}
	listRelease := &ListRelease{Releases: releases}

	keys, sortedReleases := listRelease.sortReleases()
	expectedKeys := []string{"2.0.0", "1.1.0", "1.0.0"}
	expectedUrls := []string{"url2", "url3", "url1"}

	for i, key := range keys {
		if key != expectedKeys[i] {
			t.Errorf("expected %s, got %s", expectedKeys[i], key)
		}
		if sortedReleases[i].Url != expectedUrls[i] {
			t.Errorf("expected %s, got %s", expectedUrls[i], sortedReleases[i].Url)
		}
	}
}

func Test_NewListElement_ValidInputs_ReturnsListElement(t *testing.T) {
	scope := "testScope"
	packageName := "testPackage"
	version := "1.0.0"

	element := NewListElement(scope, packageName, version)
	if element.Scope != scope || element.PackageName != packageName || element.Version != version {
		t.Errorf("expected %s, %s, %s, got %s, %s, %s", scope, packageName, version, element.Scope, element.PackageName, element.Version)
	}
}

func Test_NewUploadElement_ValidInputs_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := SourceArchive

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.Scope != scope || element.Name != name || element.Version != version || element.MimeType != mimeType {
		t.Errorf("expected %s, %s, %s, %s, got %s, %s, %s, %s", scope, name, version, mimeType, element.Scope, element.Name, element.Version, element.MimeType)
	}
}

func Test_NewUploadElement_WithFilenameOverwrite_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := SourceArchive
	filenameOverwrite := "custom"

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	element.SetFilenameOverwrite(filenameOverwrite)
	if element.FileName() != filenameOverwrite+".zip" {
		t.Errorf("expected %s, got %s", filenameOverwrite, element.FileName())
	}
}

func Test_NewUploadElement_WithExtOverwrite_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := SourceArchive
	extOverwrite := ".tar.gz"

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	element.SetExtOverwrite(extOverwrite)
	if element.FileName() != "testScope.testName-1.0.0.tar.gz" {
		t.Errorf("expected testScope.testName-1.0.0.tar.gz, got %s", element.FileName())
	}
}

func Test_NewUploadElement_WithUnsupportedUploadType_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := "unsupported"

	element := NewUploadElement(scope, name, version, mimeType, UploadElementType(uploadType))
	if element.FileName() != "testScope.testName-1.0.0.zip" {
		t.Errorf("expected testScope.testName-1.0.0.zip, got %s", element.FileName())
	}
}

func Test_NewUploadElement_WithUnknownMimetype_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "unknown"
	uploadType := SourceArchive

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "testScope.testName-1.0.0" {
		t.Errorf("expected testScope.testName-1.0.0, got %s", element.FileName())
	}
}
