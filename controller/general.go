package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
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
		err.writeResponse(w)
		return // error already logged
	}

	writeError("general error", w)
}
