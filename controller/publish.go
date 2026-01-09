package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"context"
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
		writeErrorWithStatusCode("upload failed: invalid multipart form", w, http.StatusBadRequest)
		return
	}

	var packageElement *models.UploadElement
	var storedElements []*models.UploadElement

	for {
		part, errPart := reader.NextPart()
		if errPart == io.EOF {
			slog.Debug("EOF")
			break
		}

		if part == nil {
			slog.Error("Error", "msg", err)
			writeErrorWithStatusCode("upload failed: invalid multipart form", w, http.StatusBadRequest)
			return
		}

		name := part.FormName()
		fileName := part.FileName()

		if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
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
		element, err := storeElements(w, name, scope, packageName, version, mimeType, c, part)
		if element != nil {
			storedElements = append(storedElements, element)
		}
		if err != nil {
			cleanupStoredElements(c, storedElements, scope, packageName, version)
			return // error already logged
		}

		if name == string(models.SourceArchive) {
			packageElement = element
		}
	}

	// currently we only support synchronous publishing
	// https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md#4631-synchronous-publication
	if packageElement != nil {
		// Check if Package.json is required and validate its presence
		// Note: Package.json is extracted from the source archive zip during ExtractManifestFiles
		// (called in storeElements), so it should exist here if it was in the archive
		if c.config.PackageCollections.RequirePackageJson {
			packageJsonElement := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationJson, models.PackageManifestJson)
			if !c.repo.Exists(packageJsonElement) {
				writeErrorWithStatusCode("upload failed: Package.json is required but not found in archive", w, http.StatusUnprocessableEntity)
				// Clean up all stored elements including extracted manifests
				cleanupStoredElements(c, storedElements, scope, packageName, version)
				return
			}
		}

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

// storeElements stores the given element in the repository
// returns the stored element and an error if the element could not be stored
func storeElements(w http.ResponseWriter, name string, scope string, packageName string, version string, mimeType string, c *Controller, part *multipart.Part) (*models.UploadElement, error) {
	uploadType, err := validateUploadType(name)
	if err != nil {
		writeErrorWithStatusCode(err.Error(), w, http.StatusBadRequest)
		return nil, err
	}

	element := models.NewUploadElement(scope, packageName, version, mimeType, uploadType)

	// check if file exist in repo
	if c.repo.Exists(element) {
		msg := fmt.Sprint("upload failed, package exists:", element.FileName())
		slog.Error("Error", "msg", msg)
		writeErrorWithStatusCode(msg, w, http.StatusConflict)
		return nil, fmt.Errorf("package exists: %s", element.FileName())
	}

	writer, err := c.repo.GetWriter(element)
	if err != nil {
		slog.Error("Error", "msg", err)
		writeError("upload failed, error storing file", w)
		// return element so it get cleaned up
		return element, fmt.Errorf("error storing file: %s", element.FileName())
	}

	defer func() {
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
			// return element so it get cleaned up
			return element, fmt.Errorf("error storing file: %s", element.FileName())
		}
	}

	if err := c.repo.ExtractManifestFiles(element); err != nil {
		slog.Error("Error extracting manifest files:", "error", err)
		// Continue even if extraction fails
	}

	return element, nil
}

func validateUploadType(name string) (models.UploadElementType, error) {
	switch models.UploadElementType(name) {
	case models.SourceArchive, models.SourceArchiveSignature, models.Metadata, models.MetadataSignature:
		return models.UploadElementType(name), nil
	default:
		return "", fmt.Errorf("upload failed: unsupported upload type: %s", name)
	}
}

// cleanupStoredElements removes all stored elements and extracted manifest files
// when a publish operation needs to be rolled back.
//
// Parameters:
//   - c: Controller instance with repository access
//   - storedElements: Slice of elements that were stored during upload
//   - scope: Package scope
//   - packageName: Package name
//   - version: Package version
func cleanupStoredElements(c *Controller, storedElements []*models.UploadElement, scope, packageName, version string) {
	// Remove all stored elements (metadata, signatures, source archive)
	for _, element := range storedElements {
		if err := c.repo.Remove(element); err != nil {
			slog.Warn("Failed to cleanup stored element during rollback", "element", element.FileName(), "error", err)
		}
	}

	// Remove extracted Package.swift manifest files
	// ExtractManifestFiles may have created these from the source archive
	manifestElement := models.NewUploadElement(scope, packageName, version, mimetypes.TextXSwift, models.Manifest)
	alternatives, err := c.repo.GetAlternativeManifests(manifestElement)
	if err == nil {
		for _, alt := range alternatives {
			if err := c.repo.Remove(&alt); err != nil {
				slog.Warn("Failed to cleanup manifest file during rollback", "manifest", alt.FileName(), "error", err)
			}
		}
	}

	// Also remove the base Package.swift if it exists
	if c.repo.Exists(manifestElement) {
		if err := c.repo.Remove(manifestElement); err != nil {
			slog.Warn("Failed to cleanup base manifest during rollback", "error", err)
		}
	}

	// Remove Package.json if it was extracted
	packageJsonElement := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationJson, models.PackageManifestJson)
	if c.repo.Exists(packageJsonElement) {
		if err := c.repo.Remove(packageJsonElement); err != nil {
			slog.Warn("Failed to cleanup Package.json during rollback", "error", err)
		}
	}
}
