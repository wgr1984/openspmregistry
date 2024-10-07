package models

import (
	"fmt"
	"mime"
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

func NewElement(scope string, name string, version string, mimeType string) *UploadElement {
	return &UploadElement{Scope: scope, Name: name, Version: version, MimeType: mimeType, filenameOverwrite: ""}
}

func (e *UploadElement) SetFilenameOverwrite(filename string) {
	e.filenameOverwrite = filename
}

func (e *UploadElement) SetExtOverwrite(ext string) {
	e.extOverwrite = ext
}

func (e *UploadElement) FileName() string {
	extensions, err := mime.ExtensionsByType(e.MimeType)

	if len(extensions) > 0 {
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
