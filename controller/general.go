package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/utils"
	"net/http"
)

type Controller struct {
	config       config.ServerConfig
	repo         repo.Repo
	timeProvider utils.TimeProvider
}

func NewController(config config.ServerConfig, repo repo.Repo) *Controller {
	return &Controller{
		config:       config,
		repo:         repo,
		timeProvider: utils.NewRealTimeProvider(),
	}
}

func (c *Controller) MainAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("MainAction", r)
	// 404 if no route matches
	writeErrorWithStatusCode("Not found", w, http.StatusNotFound)
}

func (c *Controller) StaticAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("FavIcon", r)
	http.ServeFile(w, r, "static"+r.URL.Path)
}
