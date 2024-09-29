package controller

import (
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"log/slog"
	"net/http"
	"regexp"
)

func MainAction(w http.ResponseWriter, r *http.Request) {

	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Request:")
		for name, values := range r.Header {
			for _, value := range values {
				slog.Debug("Header:", name, value)
			}
		}
		slog.Debug("URL", r.RequestURI)
		slog.Debug("Method", r.Method)
	}

	if err := checkHeaders(r); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	if e := writeError("general error", w); e != nil {
		log.Fatal(e)
	}
}

func PublishAction(w http.ResponseWriter, r *http.Request) {

	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Debug("Publish Request:")
		for name, values := range r.Header {
			for _, value := range values {
				slog.Debug("Header:", name, value)
			}
		}
		slog.Debug("URL", r.RequestURI)
		slog.Debug("Method", r.Method)
	}

	if err := checkHeaders(r); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	// check scope name
	params := mux.Vars(r)
	scope := params["scope"]
	packageName := params["package"]
	// version := params["version"]
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

	if e := writeError("upload failed", w); e != nil {
		log.Fatal(e)
	}
}

func writeError(msg string, w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	w.WriteHeader(http.StatusBadRequest)
	return json.NewEncoder(w).Encode(responses.Error{
		Detail: msg,
	})
}
