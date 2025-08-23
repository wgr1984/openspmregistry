package controller

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// StatusAction handles checking the status of an async operation
// GET /{scope}/{package}/{version}/status/{operation-id}
func (c *Controller) StatusAction(w http.ResponseWriter, r *http.Request) {
	printCallInfo("Status", r)
	
	// Check if async mode is enabled
	if !c.config.Async.Enabled {
		writeErrorWithStatusCode("Async mode is not enabled", w, http.StatusNotFound)
		return
	}
	
	// Get operation ID from path
	operationID := r.PathValue("operation_id")
	if operationID == "" {
		writeErrorWithStatusCode("Operation ID is required", w, http.StatusBadRequest)
		return
	}
	
	// Look up the operation
	operation, err := c.operationStore.Get(operationID)
	if err != nil {
		slog.Error("Failed to get operation", "operation_id", operationID, "error", err)
		writeError("Internal server error", w)
		return
	}
	
	if operation == nil {
		writeErrorWithStatusCode("Operation not found", w, http.StatusNotFound)
		return
	}
	
	// Return the operation status as JSON
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Version", "1")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(operation); err != nil {
		slog.Error("Failed to encode operation", "operation_id", operationID, "error", err)
	}
}