package controller

import (
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
	"net/http"
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

func (e *HeaderError) writeResponse(w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	w.WriteHeader(e.httpStatusCode)
	err := json.NewEncoder(w).Encode(responses.Error{
		Detail: e.errorMessage,
	})
	return err
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
				if !slices.Contains(supportedMediaType, mediaType) {
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
