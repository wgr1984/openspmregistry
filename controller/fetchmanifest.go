package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Controller) FetchManifestAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("FetchManifest", r)

	if err := checkHeadersEnforce(r, "swift"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	version := r.PathValue("version")

	element := models.NewUploadElement(scope, packageName, version, mimetypes.TextXSwift, models.Manifest)

	swiftVersion := r.URL.Query().Get("swift-version")
	if len(swiftVersion) > 0 {
		element.SetFilenameOverwrite(fmt.Sprintf("Package@swift-%s", swiftVersion))
	}

	filename := element.FileName()

	// load manifest Package.swift file
	reader, err := c.repo.GetReader(element)
	if err != nil {
		writeError(fmt.Sprintf("%s not found", filename), w)
		return // error already logged
	}

	defer func() {
		if reader == nil {
			return
		}
		if err := reader.Close(); err != nil {
			slog.Error("Error closing reader:", "error", err)
		}
	}()

	header := w.Header()

	if len(swiftVersion) == 0 {
		// check if alternative versions of Package.swift are present
		manifests, err := c.repo.GetAlternativeManifests(element)
		if err != nil {
			slog.Info("Alternative manifests not found:", "error", err)
		} else {
			// add alternative versions to header
			header.Set("Link", c.manifestsToString(manifests))
		}
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", mimetypes.TextXSwift)
	header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	header.Set("Cache-Control", "public, immutable")

	modDate := time.Now()
	if rawDate, err := c.repo.PublishDate(element); err == nil {
		modDate = rawDate
	} else {
		slog.Error("Error getting publish date:", "error", err)
	}
	http.ServeContent(w, r, filename, modDate, reader)
}

func (c *Controller) manifestsToString(manifests []models.UploadElement) string {
	var result string
	for i, manifest := range manifests {
		manifestFileName := manifest.FileName()
		// leave only the version number
		version := strings.Trim(manifestFileName, "Package@-.swift")
		// create the location URL the alternative Manifest can be downloaded from
		location, err := url.JoinPath(
			"https://", fmt.Sprintf("%s:%d", c.config.Hostname, c.config.Port),
			manifest.Scope,
			manifest.Name,
			manifest.Version,
			"Package.swift",
		)

		if err == nil {
			location := fmt.Sprintf("%s?swift-version=%s", location, version)

			if i != 0 {
				result += ", "
			}
			result += fmt.Sprintf("<%s>; rel=\"alternative\"; filename=\"%s\"", location, manifestFileName)

			swiftToolVersion, err2 := c.repo.GetSwiftToolVersion(&manifest)
			if err2 != nil {
				slog.Info("Swift tool version not found:", "error", err2)
			} else {
				result += fmt.Sprintf("; swift-tools-version=\"%s\"", swiftToolVersion)
			}
		}
	}
	return result
}
