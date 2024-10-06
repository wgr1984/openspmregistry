package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/responses"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
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

func writeError(msg string, w http.ResponseWriter) error {
	return writeErrorWithStatusCode(msg, w, http.StatusBadRequest)
}

func writeErrorWithStatusCode(msg string, w http.ResponseWriter, status int) error {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	header.Set("Content-Version", "1")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(responses.Error{
		Detail: msg,
	})
}
