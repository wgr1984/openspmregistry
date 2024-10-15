package models

import (
	"fmt"
	"mime"
)

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
