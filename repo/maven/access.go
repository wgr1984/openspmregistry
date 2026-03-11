package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
)

type access struct {
	client         *client
	config         config.MavenConfig
	supportsRanges *bool
	rangeCheckMu   sync.Mutex
	// metadataKeys serializes maven-metadata.xml updates per groupId/artifactId to prevent lost updates from concurrent uploads
	metadataMu   sync.Mutex
	metadataKeys map[string]*sync.Mutex
	// indexMu serializes read-modify-write of SPM registry index so concurrent publishes don't overwrite each other
	indexMu sync.Mutex
}

// mavenWriter implements io.WriteCloser and uploads data via PUT on Close()
type mavenWriter struct {
	client      *client
	config      config.MavenConfig
	access      *access
	path        string
	element     *models.UploadElement
	contentType string
	buffer      []byte
	ctx         context.Context
}

// newAccess creates a new Maven access implementation
func newAccess(client *client, cfg config.MavenConfig) *access {
	return &access{
		client:         client,
		config:         cfg,
		supportsRanges: nil,
		metadataKeys:   make(map[string]*sync.Mutex),
	}
}

// buildMavenPathForElement builds the Maven repository path for an SPM element
func (a *access) buildMavenPathForElement(element *models.UploadElement) string {
	return buildMavenPathForElement(element, a.config)
}

// checkRangeSupport checks if the Maven repository supports Range requests
func (a *access) checkRangeSupport(ctx context.Context, testPath string) (bool, error) {
	a.rangeCheckMu.Lock()
	defer a.rangeCheckMu.Unlock()

	if a.supportsRanges != nil {
		return *a.supportsRanges, nil
	}

	// Check via HEAD request for Accept-Ranges header
	// Use the provided test path or base URL
	resp, err := a.client.HEAD(ctx, testPath)
	if err != nil {
		// If HEAD fails, default to false
		supports := false
		a.supportsRanges = &supports
		return false, nil
	}

	supports := resp.Header.Get("Accept-Ranges") == "bytes"
	a.supportsRanges = &supports

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("Range support check", "supports", supports, "path", testPath)
	}

	if err := resp.Body.Close(); err != nil {
		return false, err
	}

	return supports, nil
}

// Exists checks whether element exists in the Maven repository
func (a *access) Exists(ctx context.Context, element *models.UploadElement) bool {
	path := a.buildMavenPathForElement(element)

	resp, err := a.client.HEAD(ctx, path)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// GetReader returns a reader for the specified element
func (a *access) GetReader(ctx context.Context, element *models.UploadElement) (io.ReadSeekCloser, error) {
	path := a.buildMavenPathForElement(element)

	// Check range support using the actual path
	supportsRanges, err := a.checkRangeSupport(ctx, path)
	if err != nil {
		// On error, default to buffering for safety
		supportsRanges = false
	}

	if supportsRanges {
		reader, err := newRangeReadSeekCloser(ctx, a.client, path)
		if err != nil {
			// Fall back to buffering if range requests fail
			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				slog.Debug("Range request failed, falling back to buffering", "error", err)
			}
			return newBufferedReadSeekCloser(ctx, a.client, path)
		}
		return reader, nil
	}

	// Use buffering
	return newBufferedReadSeekCloser(ctx, a.client, path)
}

// GetWriter returns a writer that uploads via PUT on Close()
func (a *access) GetWriter(ctx context.Context, element *models.UploadElement) (io.WriteCloser, error) {
	path := a.buildMavenPathForElement(element)

	return newMavenWriter(ctx, a.client, a.config, a, path, element), nil
}

func newMavenWriter(ctx context.Context, client *client, cfg config.MavenConfig, access *access, path string, element *models.UploadElement) *mavenWriter {
	return &mavenWriter{
		client:      client,
		config:      cfg,
		access:      access,
		path:        path,
		element:     element,
		contentType: element.MimeType,
		buffer:      make([]byte, 0),
		ctx:         ctx,
	}
}

// updateSPMRegistryIndex GETs the SPM registry index (spmRegistryIndexPath), adds scope and packageName if not present, sorts, and PUTs back.
// On 404 or invalid body the current list is treated as empty. Logs warnings on failure; does not fail the publish.
// Writes only packages (scopes are derived from packages keys when reading).
// Some backends (e.g. Reposilite) may not allow PUT to overwrite an existing file; we DELETE the index first then PUT so the write is a create.
func (a *access) updateSPMRegistryIndex(ctx context.Context, scope, packageName string) {
	a.indexMu.Lock()
	defer a.indexMu.Unlock()

	packages := make(map[string][]string)
	resp, err := a.client.GET(ctx, spmRegistryIndexPath)
	if err == nil && resp != nil {
		if resp.StatusCode == http.StatusOK {
			var index spmRegistryIndexResponse
			if decErr := json.NewDecoder(resp.Body).Decode(&index); decErr == nil && index.Packages != nil {
				packages = index.Packages
			}
		}
		_ = resp.Body.Close()
	}

	pkgList := packages[scope]
	pkgSeen := make(map[string]bool)
	for _, p := range pkgList {
		pkgSeen[p] = true
	}
	if !pkgSeen[packageName] {
		pkgList = append(pkgList, packageName)
		sort.Strings(pkgList)
		packages[scope] = pkgList
	}

	body, err := json.Marshal(spmRegistryIndexResponse{Packages: packages})
	if err != nil {
		slog.Warn("failed to marshal SPM registry index", "error", err)
		return
	}
	if err := a.client.DELETE(ctx, spmRegistryIndexPath); err != nil {
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			slog.Debug("DELETE index before PUT (ignore if missing)", "path", spmRegistryIndexPath, "error", err)
		}
	}
	if err := a.client.PUT(ctx, spmRegistryIndexPath, bytes.NewReader(body), "application/json"); err != nil {
		slog.Warn("failed to update SPM registry index", "path", spmRegistryIndexPath, "error", err)
	}
}

// updateMetadataLocked updates maven-metadata.xml under a per-(groupId, artifactId) lock to prevent lost updates from concurrent uploads
func (a *access) updateMetadataLocked(ctx context.Context, groupId, artifactId, version string) error {
	key := groupId + "/" + artifactId
	a.metadataMu.Lock()
	keyMu, ok := a.metadataKeys[key]
	if !ok {
		keyMu = &sync.Mutex{}
		a.metadataKeys[key] = keyMu
	}
	a.metadataMu.Unlock()

	keyMu.Lock()
	defer keyMu.Unlock()
	return updateMetadata(ctx, a.client, groupId, artifactId, version)
}

func (w *mavenWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *mavenWriter) Close() error {
	reader := bytes.NewReader(w.buffer)
	if err := w.client.PUT(w.ctx, w.path, reader, w.contentType); err != nil {
		return err
	}

	// Calculate SHA256 checksum for the uploaded data
	hash := sha256.New()
	hash.Write(w.buffer)
	checksum := fmt.Sprintf("%x", hash.Sum(nil))

	// Upload .sha256 checksum file (Maven convention)
	checksumPath := w.path + ".sha256"
	checksumReader := bytes.NewReader([]byte(checksum))
	if err := w.client.PUT(w.ctx, checksumPath, checksumReader, "text/plain"); err != nil {
		// Log warning but don't fail the upload if checksum file upload fails
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			slog.Debug("Failed to upload .sha256 checksum file", "error", err, "path", checksumPath)
		}
	}

	// Update maven-metadata.xml after successful upload of source archives
	// This ensures the metadata file is created/updated when packages are published
	// Source archives are identified by: application/zip MIME type
	if w.element.MimeType == mimetypes.ApplicationZip {
		groupId := buildGroupId(w.element.Scope, w.config)
		artifactId := buildArtifactId(w.element.Name)
		version := buildVersion(w.element.Version)
		if err := w.access.updateMetadataLocked(w.ctx, groupId, artifactId, version); err != nil {
			// Log warning but don't fail the upload if metadata update fails
			// Some repositories might not support PUT for metadata, or it might be auto-generated
			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				slog.Debug("Failed to update maven-metadata.xml", "error", err, "groupId", groupId, "artifactId", artifactId, "version", version)
			}
		}
		w.access.updateSPMRegistryIndex(w.ctx, w.element.Scope, w.element.Name)
	}

	return nil
}
