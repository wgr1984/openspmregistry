package controller

import (
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-version"
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
	for _, value := range r.Header.Values("Accept") {
		if strings.HasPrefix(value, acceptHeaderPrefix) {
			versionMediaType := strings.Split(value[len(acceptHeaderPrefix):], "+")
			if len(versionMediaType) == 2 {
				version, mediaType := versionMediaType[0], versionMediaType[1]
				if versionValue, err := strconv.Atoi(version); err != nil || versionValue != 1 {
					if err != nil {
						return NewHeaderError(fmt.Sprintf("invalid API version: %s", version))
					}
					return NewHeaderError(fmt.Sprintf("unsupported API version: %s", version))
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

	return NewHeaderError("wrong accept header")
}

func listElements(w http.ResponseWriter, c *Controller, scope string, packageName string) []models.ListElement {
	elements, err := c.repo.List(scope, packageName)
	if err != nil {
		writeError(fmt.Sprintf("error listing package %s.%s", scope, packageName), w)
	}
	if elements == nil {
		writeErrorWithStatusCode(fmt.Sprintf("error package %s.%s was not found", scope, packageName), w, http.StatusNotFound)
	}

	slices.SortFunc(elements, func(a models.ListElement, b models.ListElement) int {
		v1, err := version.NewVersion(a.Version)
		if err != nil {
			return 0
		}
		v2, err := version.NewVersion(b.Version)
		if err != nil {
			return 0
		}
		return v2.Compare(v1)
	})
	return elements
}

func addFirstReleaseAsLatest(elements []models.ListElement, c *Controller, header http.Header) {
	for i, element := range elements {
		location := locationOfElement(c, element)
		if i == 0 {
			// TODO add missing header links!!!

			// set latest element header
			//Link: <https://github.com/mona/LinkedList>; rel="canonical",
			//	<ssh://git@github.com:mona/LinkedList.git>; rel="alternate",
			//	<https://packages.example.com/mona/LinkedList/1.1.1>; rel="latest-version",
			//	<https://github.com/sponsors/mona>; rel="payment"

			header.Set("Link", fmt.Sprintf("<%s>; rel=\"latest-version\"", location))
			return
		}
	}
}

func locationOfElement(c *Controller, element models.ListElement) string {
	location, _ := url.JoinPath(
		"https://", fmt.Sprintf("%s:%d", c.config.Hostname, c.config.Port),
		element.Scope,
		element.PackageName,
		element.Version)
	return location
}
