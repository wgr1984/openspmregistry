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
	if v.Suffix == "" {
		return 1
	}
	if v1.Suffix == "" {
		return -1
	}
	return 0
}

type UploadElementType string

const (
	SourceArchive          UploadElementType = "source-archive"
	SourceArchiveSignature                   = "source-archive-signature"
	Metadata                                 = "metadata"
	MetadataSignature                        = "metadata-signature"
	Manifest                                 = "manifest"
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
		break
	case SourceArchiveSignature:
		element.SetExtOverwrite(".sig")
		break
	case Metadata:
		element.SetFilenameOverwrite("metadata")
		break
	case MetadataSignature:
		element.SetFilenameOverwrite("metadata")
		element.SetExtOverwrite(".sig")
		break
	case Manifest:
		element.SetFilenameOverwrite("Package")
		element.SetExtOverwrite(".swift")
	default:
		break
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
		return nil, nil
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
		b.Truncate(b.Len() - 1)
		b.WriteByte('}')
	}

	b.WriteByte('}')

	return b.Bytes(), nil
}

func ParseVersion(versionStr string) (*Version, error) {
	v := &Version{}
	if strings.Contains(versionStr, "-") {
		split := strings.Split(versionStr, "-")
		versionStr = split[0]
		v.Suffix = split[1]
	}
	split := strings.Split(versionStr, ".")
	if len(split) < 1 {
		return nil, errors.New("version string must have at least 1 part")
	}
	if len(split) < 2 {
		split = append(split, "0")
	}
	if len(split) < 3 {
		split = append(split, "0")
	}
	v.Major, _ = strconv.Atoi(split[0])
	v.Minor, _ = strconv.Atoi(split[1])
	v.Patch, _ = strconv.Atoi(split[2])
	return v, nil
}

func SortVersions(versions []string) []string {
	slices.SortFunc(versions, func(a string, b string) int {
		v1, err := ParseVersion(a)
		if err != nil {
			return 0
		}
		v2, err := ParseVersion(b)
		if err != nil {
			return 0
		}
		return v2.Compare(v1)
	})
	return versions
}
