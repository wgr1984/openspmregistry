// Package spm provides a Swift Package Registry proxy backend.
// In proxy mode all read operations are forwarded to an upstream SPM registry.
// In split mode (LocalPath set in config) signatures and/or package-collection
// index data can be stored and served from a local file store, enabling these
// features even when the upstream registry does not support them.
package spm

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/repo/files"
	"OpenSPMRegistry/utils"
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// SpmRepo implements the repo.Repo interface as a proxy for an upstream Swift
// Package Registry. It optionally maintains a local file store for signings
// and package-collection index data (split mode).
type SpmRepo struct {
	client       *client
	config       config.SPMConfig
	localRepo    *files.FileRepo // non-nil when LocalPath is configured (split mode)
	timeProvider utils.TimeProvider
}

// NewSpmRepo creates a new SPM proxy repository instance.
func NewSpmRepo(cfg config.SPMConfig) (*SpmRepo, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("spm: baseURL is required")
	}
	c, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("spm: failed to create HTTP client: %w", err)
	}
	var localRepo *files.FileRepo
	if cfg.LocalPath != "" {
		localRepo = files.NewFileRepo(cfg.LocalPath)
	}
	return &SpmRepo{
		client:       c,
		config:       cfg,
		localRepo:    localRepo,
		timeProvider: utils.NewRealTimeProvider(),
	}, nil
}

// isSplitMode returns true when a local file store is configured.
func (s *SpmRepo) isSplitMode() bool {
	return s.localRepo != nil
}

// useLocalForElement returns true when the element should be read from / written
// to local storage instead of being proxied to the upstream.
func (s *SpmRepo) useLocalForElement(element *models.UploadElement) bool {
	if !s.isSplitMode() {
		return false
	}
	if s.config.StoreSignings && isSigningElement(element) {
		return true
	}
	if s.config.StoreIndex && isIndexElement(element) {
		return true
	}
	return false
}

// isSigningElement returns true for source-archive and metadata signature files.
func isSigningElement(element *models.UploadElement) bool {
	return element.Extension() == "sig"
}

// isIndexElement returns true for Package.json and metadata.json sidecar files.
func isIndexElement(element *models.UploadElement) bool {
	name := element.FileName()
	return name == "Package.json" || name == "metadata.json"
}

// buildUpstreamPath returns the URL path and optional query string used to
// access the element on the upstream SPM registry.
// Returns ("", "") when the element type is not directly served by the upstream
// (e.g. signatures, which are handled locally in split mode only).
func buildUpstreamPath(element *models.UploadElement) (path string, query string) {
	if isSigningElement(element) || isIndexElement(element) {
		return "", ""
	}
	switch element.MimeType {
	case mimetypes.ApplicationZip:
		return fmt.Sprintf("/%s/%s/%s.zip", element.Scope, element.Name, element.Version), ""
	case mimetypes.TextXSwift:
		stem := element.FilenameWithoutExtension()
		if stem != "Package" && strings.HasPrefix(stem, "Package@swift-") {
			swiftVersion := strings.TrimPrefix(stem, "Package@swift-")
			return fmt.Sprintf("/%s/%s/%s/Package.swift", element.Scope, element.Name, element.Version),
				"swift-version=" + url.QueryEscape(swiftVersion)
		}
		return fmt.Sprintf("/%s/%s/%s/Package.swift", element.Scope, element.Name, element.Version), ""
	default:
		return "", ""
	}
}

// acceptForElement returns the Accept header value appropriate for the element.
func acceptForElement(element *models.UploadElement) string {
	switch element.MimeType {
	case mimetypes.ApplicationZip:
		return "application/vnd.swift.registry.v1+zip, application/zip"
	case mimetypes.TextXSwift:
		return "application/vnd.swift.registry.v1+swift, text/x-swift"
	default:
		return "application/vnd.swift.registry.v1+json"
	}
}

// bufferedReadSeekCloser wraps a bytes.Reader to implement io.ReadSeekCloser.
type bufferedReadSeekCloser struct {
	*bytes.Reader
}

func newBufferedReadSeekCloser(data []byte) *bufferedReadSeekCloser {
	return &bufferedReadSeekCloser{Reader: bytes.NewReader(data)}
}

func (b *bufferedReadSeekCloser) Close() error { return nil }

// ─── Access interface ──────────────────────────────────────────────────────

// Exists reports whether the element exists.
// For elements managed locally (split mode) the local store is consulted.
// For source archives and manifests the upstream registry is queried via HEAD.
func (s *SpmRepo) Exists(ctx context.Context, element *models.UploadElement) bool {
	if s.useLocalForElement(element) {
		return s.localRepo.Exists(ctx, element)
	}
	path, query := buildUpstreamPath(element)
	if path == "" {
		return false
	}
	resp, err := s.client.HEAD(ctx, path, query)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// GetReader returns a reader for the element.
// For elements managed locally (split mode) it reads from the local store.
// For source archives and manifests it downloads from the upstream registry and
// returns a buffered seekable reader.
func (s *SpmRepo) GetReader(ctx context.Context, element *models.UploadElement) (io.ReadSeekCloser, error) {
	if s.useLocalForElement(element) {
		return s.localRepo.GetReader(ctx, element)
	}
	path, query := buildUpstreamPath(element)
	if path == "" {
		return nil, fmt.Errorf("spm: element not available from upstream: %s", element.FileName())
	}
	resp, err := s.client.GET(ctx, path, query, acceptForElement(element))
	if err != nil {
		return nil, fmt.Errorf("spm: upstream GET failed for %s: %w", element.FileName(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("spm: failed to read upstream response for %s: %w", element.FileName(), err)
	}
	return newBufferedReadSeekCloser(data), nil
}

// GetWriter returns a writer for the element.
// Writing is only supported in split mode for element types configured for local
// storage (signatures when StoreSignings=true, index files when StoreIndex=true).
func (s *SpmRepo) GetWriter(ctx context.Context, element *models.UploadElement) (io.WriteCloser, error) {
	if s.useLocalForElement(element) {
		return s.localRepo.GetWriter(ctx, element)
	}
	return nil, fmt.Errorf("spm: writing element %q to upstream registry is not supported", element.FileName())
}

// ─── Repo interface ────────────────────────────────────────────────────────

// ExtractManifestFiles downloads the source archive from the upstream and
// extracts Package.swift / Package.json files into the local store (split mode).
// In pure proxy mode (no LocalPath) this is a no-op because there is nowhere
// to persist the extracted files.
func (s *SpmRepo) ExtractManifestFiles(ctx context.Context, element *models.UploadElement) error {
	if !s.isSplitMode() {
		return nil
	}
	reader, err := s.GetReader(ctx, element)
	if err != nil {
		return fmt.Errorf("spm: failed to get source archive for extraction: %w", err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("spm: failed to read source archive: %w", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("spm: failed to open zip: %w", err)
	}
	fileExtractor := func(name string, r io.ReadCloser) error {
		defer func() { _ = r.Close() }()
		content, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		var manifestElement *models.UploadElement
		if strings.HasPrefix(strings.ToLower(name), "package") && strings.ToLower(ext) == ".swift" {
			manifestElement = models.NewUploadElement(element.Scope, element.Name, element.Version,
				mimetypes.TextXSwift, models.Manifest)
			manifestElement.SetFilenameOverwrite(base)
		} else if strings.ToLower(name) == "package.json" {
			if !s.config.StoreIndex {
				return nil
			}
			manifestElement = models.NewUploadElement(element.Scope, element.Name, element.Version,
				mimetypes.ApplicationJson, models.PackageManifestJson)
		} else {
			return nil
		}
		writer, err := s.localRepo.GetWriter(ctx, manifestElement)
		if err != nil {
			slog.Warn("spm: failed to get writer for manifest", "name", name, "error", err)
			return nil
		}
		if _, err := writer.Write(content); err != nil {
			_ = writer.Close()
			slog.Warn("spm: failed to write manifest", "name", name, "error", err)
			return nil
		}
		if err := writer.Close(); err != nil {
			slog.Warn("spm: failed to close manifest writer", "name", name, "error", err)
		}
		return nil
	}
	return files.ExtractManifestFilesFromZipReader(element, zipReader, fileExtractor)
}

// ─── JSON response types ───────────────────────────────────────────────────

// releasesResponse is the JSON structure returned by GET /{scope}/{name}.
type releasesResponse struct {
	Releases map[string]json.RawMessage `json:"releases"`
}

// versionInfoResponse is the JSON structure returned by GET /{scope}/{name}/{version}.
type versionInfoResponse struct {
	Resources   []versionResource `json:"resources"`
	Metadata    map[string]any    `json:"metadata"`
	PublishedAt string            `json:"publishedAt"`
}

type versionResource struct {
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
}

// identifiersResponse is the JSON structure returned by GET /identifiers?url=…
type identifiersResponse struct {
	Identifiers []string `json:"identifiers"`
}

// ─── Repo methods ─────────────────────────────────────────────────────────

// List returns all versions of a package by querying the upstream registry.
func (s *SpmRepo) List(ctx context.Context, scope string, name string) ([]models.ListElement, error) {
	resp, err := s.client.GET(ctx, fmt.Sprintf("/%s/%s", scope, name), "", "")
	if err != nil {
		return []models.ListElement{}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	var releases releasesResponse
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return []models.ListElement{}, nil
	}
	var elements []models.ListElement
	for version := range releases.Releases {
		elements = append(elements, *models.NewListElement(scope, name, version))
	}
	return elements, nil
}

// EncodeBase64 returns the base64-encoded content of the element.
func (s *SpmRepo) EncodeBase64(ctx context.Context, element *models.UploadElement) (string, error) {
	if !s.Exists(ctx, element) {
		return "", fmt.Errorf("spm: element does not exist: %s", element.FileName())
	}
	reader, err := s.GetReader(ctx, element)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// getVersionInfo fetches and decodes the version information from the upstream.
func (s *SpmRepo) getVersionInfo(ctx context.Context, scope, name, version string) (*versionInfoResponse, error) {
	resp, err := s.client.GET(ctx, fmt.Sprintf("/%s/%s/%s", scope, name, version), "", "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var info versionInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("spm: failed to decode version info: %w", err)
	}
	return &info, nil
}

// PublishDate returns the publish date for the element.
// It first attempts the Last-Modified header from a HEAD request on the
// upstream, then falls back to the publishedAt field in the version info JSON.
func (s *SpmRepo) PublishDate(ctx context.Context, element *models.UploadElement) (time.Time, error) {
	path, query := buildUpstreamPath(element)
	if path != "" {
		resp, err := s.client.HEAD(ctx, path, query)
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode == http.StatusOK {
				if lm := resp.Header.Get("Last-Modified"); lm != "" {
					for _, layout := range []string{time.RFC1123, time.RFC1123Z, "Mon, 2 Jan 2006 15:04:05 MST"} {
						if t, err := time.Parse(layout, lm); err == nil {
							return t, nil
						}
					}
				}
			}
		}
	}
	info, err := s.getVersionInfo(ctx, element.Scope, element.Name, element.Version)
	if err != nil {
		return s.timeProvider.Now(), err
	}
	if info.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, info.PublishedAt); err == nil {
			return t, nil
		}
	}
	return s.timeProvider.Now(), nil
}

// Checksum returns the SHA256 checksum of the source archive as reported by
// the upstream version info endpoint.
func (s *SpmRepo) Checksum(ctx context.Context, element *models.UploadElement) (string, error) {
	info, err := s.getVersionInfo(ctx, element.Scope, element.Name, element.Version)
	if err != nil {
		return "", err
	}
	for _, r := range info.Resources {
		if r.Name == string(models.SourceArchive) {
			return r.Checksum, nil
		}
	}
	return "", errors.New("spm: checksum not found in version info")
}

// LoadMetadata returns the metadata map from the upstream version info response.
func (s *SpmRepo) LoadMetadata(ctx context.Context, scope, name, version string) (map[string]any, error) {
	info, err := s.getVersionInfo(ctx, scope, name, version)
	if err != nil {
		return nil, err
	}
	if info.Metadata == nil {
		return nil, errors.New("spm: metadata not present in version info")
	}
	return info.Metadata, nil
}

// parseLinkAlternateFilenames parses a Link header and returns the filename
// attribute values for all rel="alternate" entries.
// Example header: <https://…/Package.swift?swift-version=5.8>; rel="alternate"; filename="Package@swift-5.8.swift"
func parseLinkAlternateFilenames(linkHeader string) []string {
	var filenames []string
	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		isAlternate := false
		filename := ""
		for _, seg := range strings.Split(part, ";") {
			seg = strings.TrimSpace(seg)
			if seg == `rel="alternate"` || seg == "rel=alternate" {
				isAlternate = true
			}
			if strings.HasPrefix(seg, "filename=") {
				filename = strings.Trim(strings.TrimPrefix(seg, "filename="), `"`)
			}
		}
		if isAlternate && filename != "" {
			filenames = append(filenames, filename)
		}
	}
	return filenames
}

// GetAlternativeManifests returns the swift-version manifest variants available
// on the upstream for this package version, by inspecting the Link response
// header from a HEAD request on the default Package.swift endpoint.
func (s *SpmRepo) GetAlternativeManifests(ctx context.Context, element *models.UploadElement) ([]models.UploadElement, error) {
	path := fmt.Sprintf("/%s/%s/%s/Package.swift", element.Scope, element.Name, element.Version)
	resp, err := s.client.HEAD(ctx, path, "")
	if err != nil {
		return []models.UploadElement{}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return []models.UploadElement{}, nil
	}
	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		return []models.UploadElement{}, nil
	}
	filenames := parseLinkAlternateFilenames(linkHeader)
	var manifests []models.UploadElement
	for _, filename := range filenames {
		if filename == "Package.swift" {
			continue
		}
		if !strings.HasPrefix(filename, "Package") || filepath.Ext(filename) != ".swift" {
			continue
		}
		stem := strings.TrimSuffix(filename, ".swift")
		manifest := models.NewUploadElement(element.Scope, element.Name, element.Version,
			mimetypes.TextXSwift, models.Manifest)
		manifest.SetFilenameOverwrite(stem)
		manifests = append(manifests, *manifest)
	}
	return manifests, nil
}

// GetSwiftToolVersion extracts the swift-tools-version comment from the first
// line of the manifest file.
func (s *SpmRepo) GetSwiftToolVersion(ctx context.Context, manifest *models.UploadElement) (string, error) {
	reader, err := s.GetReader(ctx, manifest)
	if err != nil {
		return "", fmt.Errorf("spm: failed to get manifest: %w", err)
	}
	defer func() { _ = reader.Close() }()
	const prefix = "// swift-tools-version:"
	scanner := bufio.NewScanner(reader)
	if scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, prefix); ok {
			return strings.TrimSpace(after), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("spm: swift-tools-version not found in manifest")
}

// Lookup queries the upstream registry for packages associated with the given
// repository URL.
func (s *SpmRepo) Lookup(ctx context.Context, lookupURL string) []string {
	query := "url=" + url.QueryEscape(lookupURL)
	resp, err := s.client.GET(ctx, "/identifiers", query, "")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var result identifiersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return result.Identifiers
}

// Remove deletes an element from local storage (split mode only).
// In pure proxy mode this is a no-op, returning nil.
func (s *SpmRepo) Remove(ctx context.Context, element *models.UploadElement) error {
	if s.useLocalForElement(element) {
		if err := s.localRepo.Remove(ctx, element); err != nil {
			return err
		}
		return nil
	}
	return nil
}

// ListScopes returns all scopes known to the local store (split mode with
// StoreIndex=true). In pure proxy mode an empty slice is returned.
func (s *SpmRepo) ListScopes(ctx context.Context) ([]string, error) {
	if s.isSplitMode() && s.config.StoreIndex {
		return s.localRepo.ListScopes(ctx)
	}
	return []string{}, nil
}

// ListInScope returns all packages in a scope from the local store (split mode
// with StoreIndex=true). In pure proxy mode an empty slice is returned.
func (s *SpmRepo) ListInScope(ctx context.Context, scope string) ([]models.ListElement, error) {
	if s.isSplitMode() && s.config.StoreIndex {
		return s.localRepo.ListInScope(ctx, scope)
	}
	return []models.ListElement{}, nil
}

// ListAll returns all packages across all scopes from the local store (split
// mode with StoreIndex=true). In pure proxy mode an empty slice is returned.
func (s *SpmRepo) ListAll(ctx context.Context) ([]models.ListElement, error) {
	if s.isSplitMode() && s.config.StoreIndex {
		return s.localRepo.ListAll(ctx)
	}
	return []models.ListElement{}, nil
}

// LoadPackageJson loads Package.json from the local store (split mode with
// StoreIndex=true). Returns an error in pure proxy mode.
func (s *SpmRepo) LoadPackageJson(ctx context.Context, scope, name, version string) (map[string]any, error) {
	if s.isSplitMode() && s.config.StoreIndex {
		return s.localRepo.LoadPackageJson(ctx, scope, name, version)
	}
	return nil, errors.New("spm: Package.json not available in proxy mode (enable split mode with storeIndex: true)")
}
