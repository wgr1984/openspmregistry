package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/responses"
	"encoding/json"
	"fmt"
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

func writeError(msg string, w http.ResponseWriter) {
	writeErrorWithStatusCode(msg, w, http.StatusBadRequest)
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
	if slog.Default().Enabled(nil, slog.LevelDebug) {
		slog.Info(fmt.Sprintf("%s Request:", methodName))
		for name, values := range r.Header {
			for _, value := range values {
				slog.Debug("Header:", name, value)
			}
		}
		slog.Info("URL", "url", r.RequestURI)
		slog.Info("Method", "method", r.Method)
	}
}
