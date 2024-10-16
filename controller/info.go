package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/utils"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (c *Controller) InfoAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("Info", r)

	if err := checkHeadersEnforce(r, "json"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	// check scope name
	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	version := utils.StripExtension(r.PathValue("version"), ".json")

	sourceArchive := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationZip, models.SourceArchive)

	if !c.repo.Exists(sourceArchive) {
		writeErrorWithStatusCode(fmt.Sprintf("source archive %s does not exist", sourceArchive.FileName()), w, http.StatusNotFound)
		return
	}

	header := w.Header()

	addFirstReleaseAsLatest(listElements(w, c, scope, packageName), c, header)

	metadataResult := make(map[string]interface{})

	metadataResult, err := c.repo.FetchMetadata(scope, packageName, version)
	if err != nil {
		writeError("Meta data read failed", w)
		return
	}

	// encode signature
	sourceArchiveSig := utils.CopyStruct(sourceArchive)
	signatureSourceArchive, signatureSourceArchiveErr := c.repo.EncodeBase64(sourceArchiveSig.SetExtOverwrite(".sig"))
	if signatureSourceArchiveErr != nil {
		slog.Info("Signature not found:")
	}

	var signatureJson map[string]interface{}
	if len(signatureSourceArchive) > 0 {
		signatureJson = map[string]interface{}{
			"signatureBase64Encoded": signatureSourceArchive,
			"signatureFormat":        "cms-1.0.0",
		}
	} else {
		signatureJson = nil
	}

	// retrieve publish date from source archive
	dateTime, dateErr := c.repo.PublishDate(sourceArchive)
	if dateErr != nil {
		slog.Debug("Publish Date error:", dateErr)
		dateTime = time.Now()
	}
	dateString := dateTime.Format("2006-01-02T15:04:05Z")

	// retrieve checksum of source archive
	checksum, err := c.repo.Checksum(sourceArchive)
	if err != nil {
		slog.Info("Checksum error:", err)
		checksum = ""
	}

	result := map[string]interface{}{
		"id":      fmt.Sprintf("%s.%s", scope, packageName),
		"version": version,
		"resources": []interface{}{
			map[string]interface{}{
				"name":     models.SourceArchive,
				"type":     mimetypes.ApplicationZip,
				"checksum": checksum,
				"signing":  signatureJson,
			},
		},
		"metadata":    metadataResult,
		"publishedAt": dateString,
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", mimetypes.ApplicationJson)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Error("Error writing response:", "error", err)
	}
}
