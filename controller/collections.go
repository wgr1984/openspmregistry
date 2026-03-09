package controller

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/repo"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// GlobalCollectionAction handles GET /collection requests for the global collection
func (c *Controller) GlobalCollectionAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("GlobalCollection", r)

	// Check if collections are enabled
	if !c.config.PackageCollections.Enabled {
		writeErrorWithStatusCode("Package collections are not enabled", w, http.StatusNotFound)
		return
	}

	ctx := requestContext(r)
	// Get all packages
	packages, err := c.repo.ListAll(ctx)
	if err != nil {
		slog.Error("Error listing all packages", "error", err)
		writeErrorWithStatusCode("Error generating collection", w, http.StatusInternalServerError)
		return
	}

	// Generate collection
	collection, err := repo.GenerateCollection(ctx, c.repo, "", packages, c.config.Hostname)
	if err != nil {
		slog.Error("Error generating collection", "error", err)
		writeErrorWithStatusCode("Error generating collection", w, http.StatusInternalServerError)
		return
	}

	// Set headers
	data, err := json.Marshal(collection)
	if err != nil {
		slog.Error("Error marshaling collection JSON", "error", err)
		writeErrorWithStatusCode("Error generating collection", w, http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Set("Content-Type", mimetypes.ApplicationJson)
	header.Set("Content-Length", strconv.Itoa(len(data)))

	// Return collection as JSON
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Error("Error writing collection JSON", "error", err)
	}
}

// ScopeCollectionAction handles GET /collection/{scope} requests for scope-specific collections
func (c *Controller) ScopeCollectionAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("ScopeCollection", r)

	// Check if collections are enabled
	if !c.config.PackageCollections.Enabled {
		writeErrorWithStatusCode("Package collections are not enabled", w, http.StatusNotFound)
		return
	}

	ctx := requestContext(r)
	// Get scope from path
	scope := r.PathValue("scope")
	if scope == "" {
		writeErrorWithStatusCode("Scope is required", w, http.StatusBadRequest)
		return
	}

	// Get packages in scope
	packages, err := c.repo.ListInScope(ctx, scope)
	if err != nil {
		slog.Error("Error listing packages in scope", "scope", scope, "error", err)
		writeErrorWithStatusCode("Scope not found", w, http.StatusNotFound)
		return
	}

	if len(packages) == 0 {
		writeErrorWithStatusCode("No packages found in scope", w, http.StatusNotFound)
		return
	}

	// Generate collection
	collection, err := repo.GenerateCollection(ctx, c.repo, scope, packages, c.config.Hostname)
	if err != nil {
		slog.Error("Error generating collection", "scope", scope, "error", err)
		writeErrorWithStatusCode("Error generating collection", w, http.StatusInternalServerError)
		return
	}

	// Set headers
	data, err := json.Marshal(collection)
	if err != nil {
		slog.Error("Error marshaling collection JSON", "error", err)
		writeErrorWithStatusCode("Error generating collection", w, http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Set("Content-Type", mimetypes.ApplicationJson)
	header.Set("Content-Length", strconv.Itoa(len(data)))

	// Return collection as JSON
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Error("Error writing collection JSON", "error", err)
	}
}
