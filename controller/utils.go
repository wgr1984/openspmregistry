package controller

import (
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/responses"
	"OpenSPMRegistry/utils"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type HeaderError struct {
	errorMessage   string
	httpStatusCode int
}

func NewHeaderError(errorMessage string) *HeaderError {
	return &HeaderError{errorMessage: errorMessage, httpStatusCode: http.StatusBadRequest}
}

func (e *HeaderError) Error() string {
	return e.errorMessage
}

func (e *HeaderError) writeResponse(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	w.WriteHeader(e.httpStatusCode)
	err := json.NewEncoder(w).Encode(responses.Error{
		Detail: e.errorMessage,
	})
	if err != nil {
		slog.Error("Error writing response:", "error", err)
	}
}

const acceptHeaderPrefix = "application/vnd.swift.registry.v"

var supportedMediaType = []string{"json", "swift", "zip"}

// checkHeaders checks headers according to spec:
// https://github.com/swiftlang/swift-package-manager/blob/main/Documentation/PackageRegistry/Registry.md#35-api-versioning
// - version     = "1"       ; The API version
// - mediatype   =
//   - "json" /  ; JSON (default media type)
//   - "zip"  /  ; Zip archives, used for package releases
//   - "swift"   ; Swift file, used for package manifest
//
// - accept      = "application/vnd.swift.registry" [".v" version] ["+" mediatype]
func checkHeaders(r *http.Request) *HeaderError {
	return checkHeadersEnforce(r, "")
}

// checkHeadersEnforce checks headers and enforces a certain media type
func checkHeadersEnforce(r *http.Request, enforceMediaType string) *HeaderError {
	acceptHeaders := r.Header.Values("Accept")
	if len(acceptHeaders) == 0 {
		return NewHeaderError("missing Accept header")
	}

	for _, value := range acceptHeaders {
		if strings.HasPrefix(value, acceptHeaderPrefix) {
			versionMediaType := strings.Split(value[len(acceptHeaderPrefix):], "+")
			if len(versionMediaType) == 2 {
				version, mediaType := versionMediaType[0], versionMediaType[1]
				if versionValue, err := strconv.Atoi(version); err != nil || versionValue != 1 {
					if err != nil {
						return NewHeaderError(fmt.Sprintf("invalid API version: %s", version))
					}
					return &HeaderError{
						errorMessage:   fmt.Sprintf("unsupported API version: %s", version),
						httpStatusCode: http.StatusUnsupportedMediaType,
					}
				}
				if len(enforceMediaType) > 0 {
					if mediaType != enforceMediaType {
						return &HeaderError{
							errorMessage:   fmt.Sprintf("unsupported media type: %s", mediaType),
							httpStatusCode: http.StatusUnsupportedMediaType,
						}
					}
				} else if !slices.Contains(supportedMediaType, mediaType) {
					return &HeaderError{
						errorMessage:   fmt.Sprintf("unsupported media type: %s", mediaType),
						httpStatusCode: http.StatusUnsupportedMediaType,
					}
				}
				return nil
			}
		}
	}

	return &HeaderError{
		errorMessage:   "wrong accept header",
		httpStatusCode: http.StatusUnsupportedMediaType,
	}
}

func listElements(w http.ResponseWriter, c *Controller, scope string, packageName string) ([]models.ListElement, error) {
	elements, err := c.repo.List(scope, packageName)
	if err != nil {
		writeError(fmt.Sprintf("error listing package %s.%s", scope, packageName), w)
		return nil, err
	}
	if elements == nil {
		writeErrorWithStatusCode(fmt.Sprintf("error package %s.%s was not found", scope, packageName), w, http.StatusNotFound)
		return make([]models.ListElement, 0), nil
	}

	slices.SortFunc(elements, func(a models.ListElement, b models.ListElement) int {
		v1, err := models.ParseVersion(a.Version)
		if err != nil {
			return 0
		}
		v2, err := models.ParseVersion(b.Version)
		if err != nil {
			return 0
		}
		return v2.Compare(v1)
	})
	return elements, nil
}

// addLinkHeaders adds the 'latest-version', 'predecessor-version', and 'successor-version'
// Link headers based on the list of available versions for the package.
//
// Parameters:
//   - elements: list of available versions for the package
//   - currentVersion: current version of the package, if not found e.g. "", only add latest-version link
//   - c: controller
//   - header: http header
//
// Returns:
//   - void
func addLinkHeaders(elements []models.ListElement, currentVersion string, c *Controller, header http.Header) {
	var links []string
	var currentVersionIndex = -1

	// Find the index of the current version and add the latest-version link
	for i, element := range elements {
		if i == 0 {
			location := locationOfElement(c, element)
			links = append(links, fmt.Sprintf("<%s>; rel=\"latest-version\"", location))
		}
		if element.Version == currentVersion {
			currentVersionIndex = i
		}
	}

	if currentVersionIndex == -1 {
		// Current version not found in the list, only add latest-version if available
		if len(links) > 0 {
			header.Set("Link", links[0])
		}
		return
	}

	// Add predecessor-version link (element at index + 1 because list is sorted descending)
	if currentVersionIndex+1 < len(elements) {
		predecessor := elements[currentVersionIndex+1]
		location := locationOfElement(c, predecessor)
		links = append(links, fmt.Sprintf("<%s>; rel=\"predecessor-version\"", location))
	}

	// Add successor-version link (element at index - 1 because list is sorted descending)
	if currentVersionIndex-1 >= 0 {
		successor := elements[currentVersionIndex-1]
		location := locationOfElement(c, successor)
		links = append(links, fmt.Sprintf("<%s>; rel=\"successor-version\"", location))
	}

	if len(links) > 0 {
		header.Set("Link", strings.Join(links, ", "))
	}
}

func locationOfElement(c *Controller, element models.ListElement) string {
	location, _ := url.JoinPath(
		utils.BaseUrl(c.config),
		element.Scope,
		element.PackageName,
		element.Version)
	return location
}

func writeError(msg string, w http.ResponseWriter) {
	writeErrorWithStatusCode(msg, w, http.StatusInternalServerError)
}

func writeErrorWithStatusCode(msg string, w http.ResponseWriter, status int) {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	header.Set("Content-Version", "1")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(responses.Error{
		Detail: msg,
	})
	if err != nil {
		slog.Error("Error writing response:", "error", err)
	}
}

func printCallInfo(methodName string, r *http.Request) {
	if slog.Default().Enabled(context.TODO(), slog.LevelDebug) {
		slog.Info(fmt.Sprintf("%s Request:", methodName))
		for name, values := range r.Header {
			for _, value := range values {
				if name == "Authorization" {
					if strings.HasPrefix(value, "Bearer ") {
						slog.Debug("Header:", name, "Bearer "+strings.Repeat("*", 4))
					} else if strings.HasPrefix(value, "Basic ") {
						slog.Debug("Header:", name, "Basic "+strings.Repeat("*", 4))
					} else {
						slog.Debug("Header:", name, value)
					}
				} else {
					slog.Debug("Header:", name, value)
				}
			}
		}
		slog.Info("URL", "url", r.RequestURI)
		slog.Info("Method", "method", r.Method)
	}
}
