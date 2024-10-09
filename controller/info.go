package controller

import (
	"OpenSPMRegistry/models"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
)

func (c *Controller) InfoAction(w http.ResponseWriter, r *http.Request) {

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
	version := r.PathValue("version")

	sourceArchive := models.NewUploadElement(scope, packageName, version, "application/zip", models.SourceArchive)

	if !c.repo.Exists(sourceArchive) {
		if e := writeErrorWithStatusCode(fmt.Sprintf("source archive %s does not exist", sourceArchive.FileName()), w, http.StatusNotFound); e != nil {
			log.Fatal(e)
		}
		return
	}

	header := w.Header()

	addFirstReleaseAsLatest(listElements(w, c, scope, packageName), c, header)

	metadataResult := make(map[string]interface{})

	metadata := models.NewUploadElement(scope, packageName, version, "application/json", models.Metadata)
	if c.repo.Exists(metadata) {
		var b bytes.Buffer
		writer := bufio.NewWriter(&b)
		if err := c.repo.Read(metadata, writer); err != nil {
			if err := writeError("Meta data read failed", w); err != nil {
				return
			}
			return
		}
		if err := writer.Flush(); err != nil {
			if err := writeError("Meta data read flush failed", w); err != nil {
				return
			}
			return
		}

		if err := json.Unmarshal(b.Bytes(), &metadataResult); err != nil {
			if err := writeError("Meta data decode failed", w); err != nil {
				return
			}
			return
		}
	}

	result := map[string]interface{}{
		"id":      fmt.Sprintf("%s.%s", scope, packageName),
		"version": version,
		"resources": map[string]interface{}{
			"name":     "source-archive",
			"type":     "application/zip",
			"checksum": "TODO", // TODO!!!!!
			"signing": map[string]interface{}{
				"signatureBase64Encoded": "TODO", //TODO!!!!,
				"signatureFormat":        "cms-1.0.0",
			},
			"metadata":    metadataResult,
			"publishedAt": "TODO", // TODO!!!!!
		},
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Fatal(err)
	}
}
