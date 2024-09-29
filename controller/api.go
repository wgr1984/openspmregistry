package controller

import (
	"OpenSPMRegistry/responses"
	"encoding/json"
	"log"
	"net/http"
)

func MainAction(w http.ResponseWriter, r *http.Request) {
	if err := checkHeaders(r); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	if e := writeError("general error", w); e != nil {
		log.Fatal(e)
	}
}

func writeError(msg string, w http.ResponseWriter) error {
	header := w.Header()
	header.Set("Content-Type", "application/problem+json")
	header.Set("Content-Language", "en")
	w.WriteHeader(http.StatusBadRequest)
	return json.NewEncoder(w).Encode(responses.Error{
		Detail: msg,
	})
}
