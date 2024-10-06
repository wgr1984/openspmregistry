package controller

import (
	"OpenSPMRegistry/models"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
)

func (c *Controller) PublishAction(w http.ResponseWriter, r *http.Request) {

	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Info("Publish Request:")
		for name, values := range r.Header {
			for _, value := range values {
				slog.Debug("Header:", name, value)
			}
		}
		slog.Info("URL", "url", r.RequestURI)
		slog.Info("Method", "method", r.Method)
	}

	if err := checkHeaders(r); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	version := r.PathValue("version")
	if match, err := regexp.MatchString("\\A[a-zA-Z0-9](?:[a-zA-Z0-9]|-[a-zA-Z0-9]){0,38}\\z", scope); err != nil || !match {
		if e := writeError(fmt.Sprint("upload failed, incorrect scope:", scope), w); e != nil {
			log.Fatal(e)
		}
		return
	}

	if match, err := regexp.MatchString("\\A[a-zA-Z0-9](?:[a-zA-Z0-9]|[-_][a-zA-Z0-9]){0,99}\\z", packageName); err != nil || !match {
		if e := writeError(fmt.Sprint("upload failed, incorrect package:", packageName), w); e != nil {
			log.Fatal(e)
		}
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		if e := writeError("upload failed: parsing multipart form", w); e != nil {
			log.Fatal(e)
		}
		return
	}

	var packageElement *models.UploadElement

	for {
		part, errPart := reader.NextPart()
		if errPart == io.EOF {
			slog.Debug("EOF")
			break
		}

		if part == nil {
			slog.Error("Error", "msg", err)
			break
		}

		name := part.FormName()
		fileName := part.FileName()

		if slog.Default().Enabled(nil, slog.LevelDebug) {
			slog.Debug("Upload part", "name", name)
			slog.Debug("Upload part", "fileName", fileName)
			for name, values := range part.Header {
				for _, value := range values {
					slog.Debug("Upload part Header:", name, value)
				}
			}
		}

		mimeType := part.Header.Get("Content-Type")

		// currently we support only source archive storing
		unsupported, element := storeElements(w, name, scope, packageName, version, mimeType, c, part)
		if unsupported {
			return
		}

		if name == "source-archive" {
			packageElement = element
		}
	}

	// currently we only support synchronous publishing
	// https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md#4631-synchronous-publication
	if packageElement != nil {
		location, err := url.JoinPath(
			"https://", fmt.Sprintf("%s:%d", c.config.Hostname, c.config.Port),
			scope,
			packageName,
			packageElement.FileName())
		if err != nil {
			slog.Error("Error", "msg", err)
			if e := writeError("upload failed", w); e != nil {
				log.Fatal(e)
			}
		}
		header := w.Header()
		header.Set("Content-Version", "1")
		header.Set("Location", location)
		w.WriteHeader(http.StatusCreated)
		return
	}

	slog.Error("Error", "msg", "nothing found to store")
	if e := writeError("upload failed, nothing found to store", w); e != nil {
		log.Fatal(e)
	}
}

func storeElements(w http.ResponseWriter, name string, scope string, packageName string, version string, mimeType string, c *Controller, part *multipart.Part) (bool, *models.UploadElement) {
	element := models.NewElement(scope, packageName, version, mimeType)
	var filename string

	switch name {
	case "source-archive":
		filename = element.FileName()
		break
	case "metadata":
		filename = "metadata"
		element.SetFilenameOverwrite(filename)
		break
	default:
		return false, nil
	}

	// check if file exist in repo
	if c.repo.Exists(element) {
		msg := fmt.Sprint("upload failed, package exists:", filename)
		slog.Error("Error", "msg", msg)
		if e := writeErrorWithStatusCode(msg, w, http.StatusConflict); e != nil {
			log.Fatal(e)
		}
		return true, element
	}

	err := c.repo.Write(element, part)
	if err != nil {
		slog.Error("Error", "msg", err)
		if e := writeError("upload failed, error storing file", w); e != nil {
			log.Fatal(e)
		}
		return true, element
	}
	return false, element
}
