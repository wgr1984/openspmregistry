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
	"time"
)

func (c *Controller) FetchManifestAction(w http.ResponseWriter, r *http.Request) {

	printCallInfo("FetchManifest", r)

	if err := checkHeadersEnforce(r, "swift"); err != nil {
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

	// encode signature
	sourceArchiveSig := copyStruct(sourceArchive)
	signatureSourceArchive, signatureSourceArchiveErr := c.repo.EncodeBase64(sourceArchiveSig.SetExtOverwrite(".sig"))
	if signatureSourceArchiveErr != nil {
		if slog.Default().Enabled(nil, slog.LevelDebug) {
			slog.Info("Signature not found:")
		}
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
	dateString := dateTime.Format("2006-01-02T15:04:05.999Z")

	result := map[string]interface{}{
		"id":      fmt.Sprintf("%s.%s", scope, packageName),
		"version": version,
		"resources": map[string]interface{}{
			"name":        "source-archive",
			"type":        "application/zip",
			"checksum":    "TODO", // TODO!!!!!
			"signing":     signatureJson,
			"metadata":    metadataResult,
			"publishedAt": dateString,
		},
	}

	header.Set("Content-Version", "1")
	header.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Fatal(err)
	}
}
