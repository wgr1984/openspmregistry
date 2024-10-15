package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"encoding/json"
	"log"
	"net/http"
)

func (c *Controller) ListAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("List", r)

	if err := checkHeadersEnforce(r, "json"); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := stripExtension(r.PathValue("package"), ".json")

	elements := listElements(w, c, scope, packageName)

	releaseList := make(map[string]models.Release)

	header := w.Header()

	addFirstReleaseAsLatest(elements, c, header)

	for _, element := range elements {
		location := locationOfElement(c, element)
		releaseList[element.Version] = *models.NewRelease(location)
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", mimetypes.ApplicationJson)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(models.NewListRelease(releaseList)); err != nil {
		log.Fatal(err)
	}
}
