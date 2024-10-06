package models

import (
	"fmt"
	"mime"
)

type Element struct {
	Scope             string `json:"scope"`
	Name              string `json:"name"`
	Version           string `json:"version"`
	MimeType          string `json:"mime_type"`
	filenameOverwrite string
}

func NewElement(scope string, name string, version string, mimeType string) *Element {
	return &Element{Scope: scope, Name: name, Version: version, MimeType: mimeType, filenameOverwrite: ""}
}

func (e *Element) SetFilenameOverwrite(filename string) {
	e.filenameOverwrite = filename
}

func (e *Element) FileName() string {
	extensions, err := mime.ExtensionsByType(e.MimeType)

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
