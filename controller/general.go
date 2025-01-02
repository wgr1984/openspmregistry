package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
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
	printCallInfo("MainAction", r)
	// 404 if no route matches
	writeErrorWithStatusCode("Not found", w, http.StatusNotFound)
}

func (c *Controller) FavIcon(w http.ResponseWriter, r *http.Request) {
	printCallInfo("FavIcon", r)
	http.ServeFile(w, r, "static/favicon.ico")
}
