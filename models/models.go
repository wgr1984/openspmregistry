package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strconv"
	"strings"
)

// Version represents a version of a package.
// It is composed of a major, minor, patch version and an optional suffix.
type Version struct {
	Major  int
	Minor  int
	Patch  int
	Suffix string
}

func (v Version) Compare(v1 *Version) int {
	if v.Major != v1.Major {
		return v.Major - v1.Major
	}
	if v.Minor != v1.Minor {
		return v.Minor - v1.Minor
	}
	if v.Patch != v1.Patch {
		return v.Patch - v1.Patch
	}
	if v.Suffix == v1.Suffix {
		return 0
	}
	// we have a suffix
	if v.Suffix != "" {
		if v1.Suffix != "" {
			return strings.Compare(v.Suffix, v1.Suffix)
		}
		return -1
	}
	// v1 has a suffix but v not, otherwise we would not reach this point
	return 1
}

type UploadElementType string

const (
	SourceArchive          UploadElementType = "source-archive"
	SourceArchiveSignature UploadElementType = "source-archive-signature"
	Metadata               UploadElementType = "metadata"
	MetadataSignature      UploadElementType = "metadata-signature"
	Manifest               UploadElementType = "manifest"
)

type UploadElement struct {
	Scope             string `json:"scope"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	MimeType          string `json:"mime_type"`
	filenameOverwrite string
	extOverwrite      string
}

type ListElement struct {
	Scope       string
	PackageName string
	Version     string
}

func NewListElement(scope string, packageName string, version string) *ListElement {
	return &ListElement{Scope: scope, PackageName: packageName, Version: version}
}

func NewUploadElement(scope string, name string, version string, mimeType string, uploadType UploadElementType) *UploadElement {
	element := &UploadElement{Scope: scope, Name: name, Version: version, MimeType: mimeType, filenameOverwrite: ""}

	switch uploadType {
	case SourceArchive:
		// No overwrite needed
	case SourceArchiveSignature:
		element.SetExtOverwrite(".sig")
	case Metadata:
		element.SetFilenameOverwrite("metadata")
	case MetadataSignature:
		element.SetFilenameOverwrite("metadata")
		element.SetExtOverwrite(".sig")
	case Manifest:
		element.SetFilenameOverwrite("Package")
		element.SetExtOverwrite(".swift")
	default:
		// No overwrite needed
	}

	return element
}

func (e *UploadElement) SetFilenameOverwrite(filename string) *UploadElement {
	e.filenameOverwrite = filename
	return e
}

func (e *UploadElement) SetExtOverwrite(ext string) *UploadElement {
	e.extOverwrite = ext
	return e
}

func (e *UploadElement) FileName() string {
	extensions, err := mime.ExtensionsByType(e.MimeType)

	if len(e.extOverwrite) > 0 {
		if len(extensions) > 0 {
			extensions[0] = e.extOverwrite
		} else {
			extensions = []string{e.extOverwrite}
		}
	}

	if err != nil || extensions == nil || len(extensions) == 0 {
		if len(e.filenameOverwrite) > 0 {
			return e.filenameOverwrite
		}
		return fmt.Sprintf("%s.%s-%s", e.Scope, e.Name, e.Version)
	}

	if len(e.filenameOverwrite) > 0 {
		return fmt.Sprintf("%s%s", e.filenameOverwrite, extensions[0])
	}
	return fmt.Sprintf("%s.%s-%s%s", e.Scope, e.Name, e.Version, extensions[0])
}

type Release struct {
	Url string `json:"url"`
}

func NewRelease(url string) *Release {
	return &Release{Url: url}
}

type ListRelease struct {
	Releases map[string]Release `json:"releases"`
}

func NewListRelease(releases map[string]Release) *ListRelease {
	return &ListRelease{Releases: releases}
}

func (l *ListRelease) sortReleases() ([]string, []Release) {

	keys := make([]string, 0, len(l.Releases))
	for key := range l.Releases {
		keys = append(keys, key)
	}

	keys = SortVersions(keys)

	sortedReleases := make([]Release, len(l.Releases))
	for i, key := range keys {
		sortedReleases[i] = l.Releases[key]
	}

	return keys, sortedReleases
}

func (l *ListRelease) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer

	if l == nil {
		b.WriteString("null")
		return b.Bytes(), nil
	}

	b.WriteByte('{')

	b.WriteString("\"releases\":")
	if l.Releases == nil {
		b.WriteString("null")
	} else {
		b.WriteByte('{')
		keys, sortedReleases := l.sortReleases()
		for i, key := range keys {
			b.WriteString("\"")
			b.WriteString(key)
			b.WriteString("\":")
			data, err := json.Marshal(sortedReleases[i])
			if err != nil {
				b.WriteString("null")
			} else {
				b.Write(data)
			}
			b.WriteByte(',')
		}
		if len(keys) > 0 {
			b.Truncate(b.Len() - 1)
		}
		b.WriteByte('}')
	}

	b.WriteByte('}')

	return b.Bytes(), nil
}

// ParseVersion parses a version string and returns a Version struct
// If the version string contains a suffix, it is stored in the Version struct
// as well.
// The version string must have at least 1 part.
// If the version string has less than 2 parts, the missing parts are set to 0.
// In case of invalid version string, an error is returned.
func ParseVersion(versionStr string) (*Version, error) {
	v := &Version{}
	if strings.Contains(versionStr, "-") {
		split := strings.Split(versionStr, "-")
		if len(split) > 2 {
			return nil, errors.New("version string must have at only 1 suffix")
		}
		versionStr = split[0]
		v.Suffix = split[1]
		if v.Suffix == "" {
			return nil, errors.New("suffix cannot be empty once specified")
		}
	}
	split := strings.Split(versionStr, ".")
	// as we have at least 1 part, no need to check for empty array

	if len(split) < 2 {
		split = append(split, "0")
	}
	if len(split) < 3 {
		split = append(split, "0")
	}

	var err error

	if v.Major, err = strconv.Atoi(split[0]); err != nil {
		return nil, err
	}
	if v.Minor, err = strconv.Atoi(split[1]); err != nil {
		return nil, err
	}
	if v.Patch, err = strconv.Atoi(split[2]); err != nil {
		return nil, err
	}
	return v, nil
}

func SortVersions(versions []string) []string {
	slices.SortFunc(versions, func(a string, b string) int {
		v1, err := ParseVersion(a)
		if err != nil {
			return 1
		}
		v2, err := ParseVersion(b)
		if err != nil {
			return -1
		}
		return v2.Compare(v1)
	})
	return versions
}
