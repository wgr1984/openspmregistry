package maven

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
)

type access struct {
	client        *client
	config        config.MavenConfig
	supportsRanges *bool
	rangeCheckMu  sync.Mutex
}

// newAccess creates a new Maven access implementation
func newAccess(client *client, cfg config.MavenConfig) *access {
	return &access{
		client:        client,
		config:        cfg,
		supportsRanges: nil,
	}
}

// buildMavenPathForElement builds the Maven repository path for an SPM element
func (a *access) buildMavenPathForElement(element *models.UploadElement) (string, error) {
	groupId := buildGroupId(element.Scope, a.config)
	artifactId := buildArtifactId(element.Name)
	version := buildVersion(element.Version)

	// Determine extension from filename
	filename := element.FileName()
	ext := ""
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		ext = filename[idx:]
	} else {
		// Try to get extension from MIME type
		extensions, err := mime.ExtensionsByType(element.MimeType)
		if err == nil && len(extensions) > 0 {
			ext = extensions[0]
		} else {
			ext = ".zip" // Default
		}
	}

	// Handle sidecar files (metadata.json, Package.swift, Package.json)
	// These are stored with classifier suffix (using lowercase per Maven conventions)
	if strings.HasPrefix(strings.ToLower(filename), "metadata") {
		// metadata.json or metadata.sig
		if strings.HasSuffix(filename, ".json") {
			return buildMavenPath(groupId, artifactId, version, "metadata", ".json"), nil
		}
		if strings.HasSuffix(filename, ".sig") {
			return buildMavenPath(groupId, artifactId, version, "metadata", ".sig"), nil
		}
	}
	if strings.HasPrefix(strings.ToLower(filename), "package") {
		if strings.HasSuffix(filename, ".swift") {
			// Check for alternative manifests like Package@swift-5.0.swift
			if strings.Contains(filename, "@") {
				// Extract the swift version part
				// Format: Package@swift-5.7.0.swift -> extract "5.7.0"
				parts := strings.Split(filename, "@")
				if len(parts) == 2 {
					swiftPart := strings.TrimSuffix(parts[1], ".swift")
					// Remove "swift-" prefix if present to get just the version
					swiftVersion := strings.TrimPrefix(swiftPart, "swift-")
					// Use Maven-compliant format: package-swift-5.7.0
					// (lowercase, hyphens only - no @ symbol or uppercase per Maven conventions)
					// Multiple dashes in classifier are valid (e.g., "test-jar", "sources-javadoc")
					classifier := "package-swift-" + swiftVersion
					return buildMavenPath(groupId, artifactId, version, classifier, ".swift"), nil
				}
			}
			return buildMavenPath(groupId, artifactId, version, "package", ".swift"), nil
		}
		if strings.HasSuffix(filename, ".json") {
			return buildMavenPath(groupId, artifactId, version, "package", ".json"), nil
		}
	}

	// Main artifact (source-archive.zip) - no classifier
	return buildMavenPath(groupId, artifactId, version, "", ext), nil
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
	defer resp.Body.Close()

	supports := resp.Header.Get("Accept-Ranges") == "bytes"
	a.supportsRanges = &supports

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("Range support check", "supports", supports, "path", testPath)
	}

	return supports, nil
}

// Exists checks whether element exists in the Maven repository
func (a *access) Exists(ctx context.Context, element *models.UploadElement) bool {
	path, err := a.buildMavenPathForElement(element)
	if err != nil {
		return false
	}

	resp, err := a.client.HEAD(ctx, path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetReader returns a reader for the specified element
func (a *access) GetReader(ctx context.Context, element *models.UploadElement) (io.ReadSeekCloser, error) {
	path, err := a.buildMavenPathForElement(element)
	if err != nil {
		return nil, fmt.Errorf("failed to build Maven path: %w", err)
	}

	// Check range support using the actual path
	supportsRanges, err := a.checkRangeSupport(ctx, path)
	if err != nil {
		// On error, default to buffering for safety
		supportsRanges = false
	}

	if supportsRanges {
		reader, err := newRangeReadSeekCloser(a.client, path, ctx)
		if err != nil {
			// Fall back to buffering if range requests fail
			if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
				slog.Debug("Range request failed, falling back to buffering", "error", err)
			}
			return newBufferedReadSeekCloser(a.client, path, ctx)
		}
		return reader, nil
	}

	// Use buffering
	return newBufferedReadSeekCloser(a.client, path, ctx)
}

// GetWriter returns a writer that uploads via PUT on Close()
func (a *access) GetWriter(ctx context.Context, element *models.UploadElement) (io.WriteCloser, error) {
	path, err := a.buildMavenPathForElement(element)
	if err != nil {
		return nil, fmt.Errorf("failed to build Maven path: %w", err)
	}

	return newMavenWriter(a.client, path, element.MimeType, ctx), nil
}

// mavenWriter implements io.WriteCloser and uploads data via PUT on Close()
type mavenWriter struct {
	client      *client
	path        string
	contentType string
	buffer      []byte
	ctx         context.Context
}

func newMavenWriter(client *client, path, contentType string, ctx context.Context) *mavenWriter {
	return &mavenWriter{
		client:      client,
		path:        path,
		contentType: contentType,
		buffer:      make([]byte, 0),
		ctx:         ctx,
	}
}

func (w *mavenWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *mavenWriter) Close() error {
	reader := bytes.NewReader(w.buffer)
	return w.client.PUT(w.ctx, w.path, reader, w.contentType)
}
