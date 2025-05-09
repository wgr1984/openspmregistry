package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"encoding/json"
	"log/slog"
	"net/http"
)

func (c *Controller) ListAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("List", r)

	if err := checkHeadersEnforce(r, "json"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := utils.StripExtension(r.PathValue("package"), ".json")

	elements, err := listElements(w, c, scope, packageName)
	if err != nil {
		return // error already logged
	}

	releaseList := make(map[string]models.Release)

	header := w.Header()

	// For list view, only add latest-version link. Pass empty currentVersion.
	addLinkHeaders(elements, "", c, header)

	for _, element := range elements {
		location := locationOfElement(c, element)
		releaseList[element.Version] = *models.NewRelease(location)
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", mimetypes.ApplicationJson)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(models.NewListRelease(releaseList)); err != nil {
		slog.Error("Error encoding JSON:", "error", err)
	}
}
