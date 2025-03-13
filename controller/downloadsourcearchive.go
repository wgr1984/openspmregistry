package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func (c *Controller) DownloadSourceArchiveAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("DownloadSourceArchive", r)

	if err := checkHeadersEnforce(r, "zip"); err != nil {
		err.writeResponse(w)
		return // error already logged
	}

	scope := r.PathValue("scope")
	packageName := r.PathValue("package")
	versionRaw := r.PathValue("version")
	version := strings.TrimSuffix(versionRaw, filepath.Ext(versionRaw))

	element := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationZip, models.SourceArchive)

	if !c.repo.Exists(element) {
		writeErrorWithStatusCode(fmt.Sprintf("source archive %s does not exist", element.FileName()), w, http.StatusNotFound)
		return
	}

	header := w.Header()
	header.Set("Content-Type", mimetypes.ApplicationZip)
	header.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", element.FileName()))
	header.Set("Content-Version", "1")
	header.Set("Cache-Control", "public, immutable")
	checksum, err := c.repo.Checksum(element)
	if err != nil {
		slog.Error("Error calculating checksum:", "error", err)
	} else {
		header.Set("Digest", fmt.Sprintf("sha-256=%s", checksum))
	}

	signatureElement := models.NewUploadElement(scope, packageName, version, mimetypes.ApplicationOctetStream, models.SourceArchiveSignature)
	if c.repo.Exists(signatureElement) {
		signature, err := c.repo.EncodeBase64(signatureElement)
		if err != nil {
			slog.Info("Signature not found:")
		} else {
			header.Set("X-Swift-Package-Signature-Format", "cms-1.0.0")
			header.Set("X-Swift-Package-Signature", signature)
		}
	}

	header.Set("Accept-Ranges", "bytes")

	reader, err := c.repo.GetReader(element)
	if err != nil {
		writeError(fmt.Sprintf("error reading source archive %s", element.FileName()), w)
		return // error already logged
	}
	defer func() {
		if err := reader.Close(); err != nil {
			slog.Error("Error closing reader:", "error", err)
		}
	}()
	// Handle byte range requests
	http.ServeContent(w, r, element.FileName(), time.Now(), reader)
}
