package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

func (c *Controller) LookupAction(w http.ResponseWriter, r *http.Request) {
	if err := checkHeadersEnforce(r, "json"); err != nil {
		err.writeResponse(w)
		return
	}

	url := r.URL.Query().Get("url")
	if url == "" {
		writeError("url is required", w)
		return
	}

	identifiers := c.repo.Lookup(url)

	if identifiers == nil {
		writeErrorWithStatusCode(fmt.Sprintf("%s not found", url), w, http.StatusNotFound)
		return
	}

	header := w.Header()
	header.Set("Content-Version", "1")
	header.Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"identifiers": identifiers,
	}); err != nil {
		slog.Error("Error encoding JSON:", "error", err)
	}
}
