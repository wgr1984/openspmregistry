package controller

import (
	"OpenSPMRegistry/collectionsign"
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/utils"
	"net/http"
)

type Controller struct {
	config             config.ServerConfig
	repo               repo.Repo
	timeProvider       utils.TimeProvider
	collectionSigner   *collectionsign.Signer
}

// ControllerOption configures NewController.
type ControllerOption func(*Controller)

// WithCollectionSigner enables SwiftPM signed package collections (JWS) on GET /collection responses.
func WithCollectionSigner(s *collectionsign.Signer) ControllerOption {
	return func(c *Controller) {
		c.collectionSigner = s
	}
}

func NewController(config config.ServerConfig, repo repo.Repo, opts ...ControllerOption) *Controller {
	c := &Controller{
		config:       config,
		repo:         repo,
		timeProvider: utils.NewRealTimeProvider(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
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
