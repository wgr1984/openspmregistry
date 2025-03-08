package models

import (
	"OpenSPMRegistry/mimetypes"
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

func Test_NewRelease_ValidInputs_ReturnsRelease(t *testing.T) {
	url := "http://example.com"
	release := NewRelease(url)

	if release.Url != url {
		t.Errorf("expected %s, got %s", url, release.Url)
	}
}

func Test_NewListRelease_ValidInputs_ReturnsListRelease(t *testing.T) {
	releases := map[string]Release{
		"1.0.0": {Url: "url1"},
		"2.0.0": {Url: "url2"},
	}
	listRelease := NewListRelease(releases)

	if len(listRelease.Releases) != 2 {
		t.Errorf("expected 2, got %d", len(listRelease.Releases))
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

func Test_ParseVersion_ValidVersionStringWithSuffix_ReturnsVersion(t *testing.T) {
	versionStr := "1.2.3-alpha"
	expected := &Version{Major: 1, Minor: 2, Patch: 3, Suffix: "alpha"}

	result, err := ParseVersion(versionStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *result != *expected {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func Test_ParseVersion_InvalidMajorVersion_ReturnsError(t *testing.T) {
	versionStr := "invalid.2.3"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ParseVersion_InvalidMinorVersion_ReturnsError(t *testing.T) {
	versionStr := "1.invalid.3"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ParseVersion_InvalidPatchVersion_ReturnsError(t *testing.T) {
	versionStr := "1.2.invalid"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ParseVersion_InvalidSuffix_ReturnsError(t *testing.T) {
	versionStr := "1.2.3-"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ParseVersion_MultipleSuffixes_ReturnsError(t *testing.T) {
	versionStr := "1.2.3-alpha-beta"

	_, err := ParseVersion(versionStr)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

func Test_ParseVersion_NoParts_ReturnsError(t *testing.T) {
	versionStrings := []string{"dsfsdf", ""}

	for _, versionStr := range versionStrings {
		_, err := ParseVersion(versionStr)
		if err == nil {
			t.Errorf("expected error, got nil")
		}
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

func Test_FileName_WithExtOverwrite_UnKnownMimeType_ReturnsOverwrittenExt(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "unknown"}
	element.SetExtOverwrite(".tar.gz")
	expected := "scope.name-1.0.0.tar.gz"

	result := element.FileName()
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_FileName_WithExtOverwrite_UnKnownMimeType_ReturnsDefaultName(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "unknown"}
	expected := "scope.name-1.0.0"

	result := element.FileName()
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func Test_FileName_WithExtOverwrite_UnKnownMimeType_FilenameOverwrite_ReturnsOverwrittenName(t *testing.T) {
	element := &UploadElement{Scope: "scope", Name: "name", Version: "1.0.0", MimeType: "unknown"}
	element.SetFilenameOverwrite("custom")
	expected := "custom"

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

func Test_SortVersions_ValidVersionsWithSuffix_ReturnsSorted(t *testing.T) {
	versions := []string{"1.0.0-beta", "1.0.0-alpha", "1.0.0"}
	expected := []string{"1.0.0", "1.0.0-beta", "1.0.0-alpha"}

	result := SortVersions(versions)
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected %s, got %s", expected[i], v)
		}
	}
}

func Test_SortVersions_ValidVersionsWithSnapshot_ReturnsSorted(t *testing.T) {
	versions := []string{"1.0.0", "1.0.1", "1.0.0-snapshot"}
	expected := []string{"1.0.1", "1.0.0", "1.0.0-snapshot"}

	result := SortVersions(versions)
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected at post %d %s, got %s", i, expected[i], v)
		}
	}
}

func Test_SortVersions_NoVersions_ReturnsEmpty(t *testing.T) {
	versions := []string{}

	result := SortVersions(versions)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func Test_SortVersions_ContainsInvalidVersions_ReturnsPartlyUnsorted(t *testing.T) {
	versions := []string{"1.0.0", "non-valid", "invalid", "2.0.0", "not-a-version"}
	expected := []string{"2.0.0", "1.0.0", "non-valid", "invalid", "not-a-version"}

	result := SortVersions(versions)
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("expected #%d: %s, got %s", i, expected[i], v)
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

func Test_MarshalJSON_EmptyReleases_ListNil_ReturnsEmptyObject(t *testing.T) {
	listRelease := &ListRelease{Releases: nil}

	data, err := listRelease.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"releases":null}`
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

func Test_sortReleases_ValidReleases_EmptyReleases_ReturnsEmptyObject(t *testing.T) {
	listRelease := &ListRelease{Releases: map[string]Release{}}

	keys, sortedReleases := listRelease.sortReleases()
	if len(keys) != 0 || len(sortedReleases) != 0 {
		t.Errorf("expected empty, got %v, %v", keys, sortedReleases)
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

func Test_NewUploadElement_UploadTypeSourceArchive_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := SourceArchive

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "testScope.testName-1.0.0.zip" {
		t.Errorf("expected testScope.testName-1.0.0.zip, got %s", element.FileName())
	}
}

func Test_NewUploadElement_UploadTypeSourceArchiveSignature_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := SourceArchiveSignature

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "testScope.testName-1.0.0.sig" {
		t.Errorf("expected testScope.testName-1.0.0.sig, got %s", element.FileName())
	}
}

func Test_NewUploadElement_UploadTypeMetadata_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := Metadata

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "metadata.zip" {
		t.Errorf("expected metadata, got %s", element.FileName())
	}
}

func Test_NewUploadElement_UploadTypeMetadataSignature_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := "application/zip"
	uploadType := MetadataSignature

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "metadata.sig" {
		t.Errorf("expected metadata.sig, got %s", element.FileName())
	}
}

func Test_NewUploadElement_UploadTypeManifest_ReturnsUploadElement(t *testing.T) {
	scope := "testScope"
	name := "testName"
	version := "1.0.0"
	mimeType := mimetypes.TextXSwift
	uploadType := Manifest

	element := NewUploadElement(scope, name, version, mimeType, uploadType)
	if element.FileName() != "Package.swift" {
		t.Errorf("expected Package.swift, got %s", element.FileName())
	}
}
