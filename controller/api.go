package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"log/slog"
	"net/http"
	"regexp"
)

type Controller struct {
	config config.ServerConfig
	repo   repo.Repo
}

func NewController(config config.ServerConfig, repo repo.Repo) *Controller {
	return &Controller{config: config, repo: repo}
}

func (c *Controller) MainAction(w http.ResponseWriter, r *http.Request) {

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
	params := mux.Vars(r)
	scope := params["scope"]
	packageName := params["package"]
	version := params["version"]
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

	// check if file exist in repo
	if c.repo.Exists(scope, packageName, version) {
		if e := writeError(fmt.Sprint("upload failed, package exists:", scope, '.', packageName, '@', version), w); e != nil {
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

		err := c.repo.Write(scope, packageName, version, part)
		if err != nil {
			if e := writeError("upload failed, error storing file", w); e != nil {
				log.Fatal(e)
			}
			return
		}
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
