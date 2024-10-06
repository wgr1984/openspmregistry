package controller

import (
	"OpenSPMRegistry/models"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/hashicorp/go-version"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
)

func (c *Controller) ListAction(w http.ResponseWriter, r *http.Request) {

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

	elements, err := c.repo.List(scope, packageName)
	if err != nil {
		if e := writeError(fmt.Sprintf("error listing package %s.%s", scope, packageName), w); e != nil {
			log.Fatal(e)
		}
	}
	if elements == nil {
		if e := writeErrorWithStatusCode(fmt.Sprintf("error package %s.%s was not found", scope, packageName), w, http.StatusNotFound); e != nil {
			log.Fatal(e)
		}
	}

	slices.SortFunc(elements, func(a models.ListElement, b models.ListElement) int {
		v1, err := version.NewVersion(a.Version)
		if err != nil {
			return 0
		}
		v2, err := version.NewVersion(b.Version)
		if err != nil {
			return 0
		}
		return v2.Compare(v1)
	})

	releaseList := make(map[string]models.Release)

	header := w.Header()

	for i, element := range elements {
		location, _ := url.JoinPath(
			"https://", fmt.Sprintf("%s:%d", c.config.Hostname, c.config.Port),
			element.Scope,
			element.PackageName,
			element.Version)
		if i == 0 {
			// TODO add missing header links!!!

			// set latest element header
			//Link: <https://github.com/mona/LinkedList>; rel="canonical",
			//	<ssh://git@github.com:mona/LinkedList.git>; rel="alternate",
			//	<https://packages.example.com/mona/LinkedList/1.1.1>; rel="latest-version",
			//	<https://github.com/sponsors/mona>; rel="payment"

			header.Set("Link", fmt.Sprintf("<%s>; rel=\"latest-version\"", location))
		}
		releaseList[element.Version] = *models.NewRelease(location)
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(models.NewListRelease(releaseList)); err != nil {
		log.Fatal(err)
	}
}
