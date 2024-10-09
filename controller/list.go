package controller

import (
	"OpenSPMRegistry/models"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
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
	scope := r.PathValue("scope")
	packageName := r.PathValue("package")

	elements := listElements(w, c, scope, packageName)

	releaseList := make(map[string]models.Release)

	header := w.Header()

	addFirstReleaseAsLatest(elements, c, header)

	for _, element := range elements {
		location := locationOfElement(c, element)
		releaseList[element.Version] = *models.NewRelease(location)
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(models.NewListRelease(releaseList)); err != nil {
		log.Fatal(err)
	}
}
