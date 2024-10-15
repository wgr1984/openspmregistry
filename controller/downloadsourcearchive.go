package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
)

func (c *Controller) DownloadSourceArchiveAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("DownloadSourceArchive", r)

	if err := checkHeadersEnforce(r, "zip"); err != nil {
		if e := err.writeResponse(w); e != nil {
			log.Fatal(e)
		}
		return
	}

	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	versionRaw := r.PathValue("version")
	version := strings.TrimSuffix(versionRaw, filepath.Ext(versionRaw))

	element := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationZip, models.SourceArchive)

	if !c.repo.Exists(element) {
		if e := writeErrorWithStatusCode(fmt.Sprintf("source archive %s does not exist", element.FileName()), w, http.StatusNotFound); e != nil {
			if slog.Default().Enabled(nil, slog.LevelDebug) {
				slog.Debug("Error writing response:", "error", e)
			}
		}
		return
	}

	header := w.Header()
	header.Set("Content-Type", mimetypes.ApplicationZip)
	header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", element.FileName()))
	header.Set("Content-Version", "1")
	header.Set("Cache-Control", "public, immutable")
	checksum, err := c.repo.Checksum(element)
	if err != nil {
		if slog.Default().Enabled(nil, slog.LevelDebug) {
			slog.Info("Error calculating checksum:", "error", err)
		}
	} else {
		header.Set("Digest", fmt.Sprintf("sha-256=%s", checksum))
	}

	signatureElement := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationOctetStream, models.SourceArchiveSignature)
	if c.repo.Exists(signatureElement) {
		signature, err := c.repo.EncodeBase64(signatureElement)
		if err != nil {
			if slog.Default().Enabled(nil, slog.LevelDebug) {
				slog.Info("Signature not found:")
			}
		} else {
			header.Set("X-Swift-Package-Signature-Format", "cms-1.0.0")
			header.Set("X-Swift-Package-Signature", signature)
		}
	}

	// TODO add support for header.Set("Accept-Ranges", "bytes")
	err2 := c.repo.Read(element, w)
	if err2 != nil {
		if e := writeError(fmt.Sprintf("error reading source archive %s", element.FileName()), w); e != nil {
			return // error already logged
		}
		return // error already logged
	}
}
