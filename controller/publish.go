package controller

import (
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
)

func (c *Controller) PublishAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("Publish", r)

	if err := checkHeadersEnforce(r, "json"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	version := r.PathValue("version")
	if match, err := regexp.MatchString("\\A[a-zA-Z0-9](?:[a-zA-Z0-9]|-[a-zA-Z0-9]){0,38}\\z", scope); err != nil || !match {
		writeErrorWithStatusCode(fmt.Sprint("upload failed, incorrect scope:", scope), w, http.StatusBadRequest)
		return
	}

	if match, err := regexp.MatchString("\\A[a-zA-Z0-9](?:[a-zA-Z0-9]|[-_][a-zA-Z0-9]){0,99}\\z", packageName); err != nil || !match {
		writeErrorWithStatusCode(fmt.Sprint("upload failed, incorrect package:", packageName), w, http.StatusBadRequest)
		return
	}

	reader, err := r.MultipartReader()
	if err != nil {
		writeError("upload failed: parsing multipart form", w)
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
			utils.BaseUrl(c.config),
			scope,
			packageName,
			packageElement.FileName())
		if err != nil {
			slog.Error("Error", "msg", err)
			writeError("upload failed", w)
		}
		header := w.Header()
		header.Set("Content-Version", "1")
		header.Set("Location", location)
		w.WriteHeader(http.StatusCreated)
		return
	}

	slog.Error("Error", "msg", "nothing found to store")
	writeError("upload failed, nothing found to store", w)
}

func storeElements(w http.ResponseWriter, name string, scope string, packageName string, version string, mimeType string, c *Controller, part *multipart.Part) (bool, *models.UploadElement) {
	uploadType := models.UploadElementType(name)
	element := models.NewUploadElement(scope, packageName, version, mimeType, uploadType)

	switch uploadType {
	case models.SourceArchive:
	case models.SourceArchiveSignature:
	case models.Metadata:
	case models.MetadataSignature:
		break
	default:
		return false, nil
	}

	// check if file exist in repo
	if c.repo.Exists(element) {
		msg := fmt.Sprint("upload failed, package exists:", element.FileName())
		slog.Error("Error", "msg", msg)
		writeErrorWithStatusCode(msg, w, http.StatusConflict)
		return true, element
	}

	writer, err := c.repo.GetWriter(element)
	if err != nil {
		slog.Error("Error", "msg", err)
		writeError("upload failed, error storing file", w)
		return true, element
	}

	defer func() {
		if writer == nil {
			return
		}
		if err := writer.Close(); err != nil {
			slog.Error("Error closing writer:", "error", err)
		}
	}()

	_, err1 := io.Copy(writer, part)
	errs := []error{
		err1,
		part.Close(),
	}

	for _, err := range errs {
		if err != nil {
			slog.Error("Error", "msg", err)
			writeError("upload failed, error storing file", w)
			return true, element
		}
	}

	if err := c.repo.ExtractManifestFiles(element); err != nil {
		return false, nil
	}

	return false, element
}
