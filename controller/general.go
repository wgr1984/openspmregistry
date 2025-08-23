package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/utils"
	"net/http"
	"time"
)

type Controller struct {
	config          config.ServerConfig
	repo            repo.Repo
	timeProvider    utils.TimeProvider
	asyncProcessor  *AsyncProcessor
	operationStore  models.AsyncOperationStore
}

func NewController(config config.ServerConfig, repo repo.Repo) *Controller {
	c := &Controller{
		config:         config,
		repo:           repo,
		timeProvider:   utils.NewRealTimeProvider(),
		operationStore: models.NewInMemoryOperationStore(),
	}
	
	// Initialize async processor if enabled
	if config.Async.Enabled {
		workers := config.Async.Workers
		if workers <= 0 {
			workers = 4 // Default to 4 workers
		}
		c.asyncProcessor = NewAsyncProcessor(workers, c.operationStore, 1*time.Hour)
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

// Shutdown gracefully shuts down the controller and its async processor
func (c *Controller) Shutdown() {
	if c.asyncProcessor != nil {
		c.asyncProcessor.Shutdown()
	}
}
