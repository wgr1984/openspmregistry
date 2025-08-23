package controller

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/utils"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"
)

// AsyncPublishJob represents a job for async package publishing
type AsyncPublishJob struct {
	Operation *models.AsyncOperation
	Element   *models.UploadElement
	Config    config.ServerConfig
	Repo      repo.Repo
}

// AsyncProcessor handles background processing of packages
type AsyncProcessor struct {
	jobs        chan *AsyncPublishJob
	workers     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	store       models.AsyncOperationStore
	cleanupTick *time.Ticker
}

// NewAsyncProcessor creates a new async processor
func NewAsyncProcessor(workers int, store models.AsyncOperationStore, cleanupInterval time.Duration) *AsyncProcessor {
	ctx, cancel := context.WithCancel(context.Background())
	
	p := &AsyncProcessor{
		jobs:        make(chan *AsyncPublishJob, workers*10), // Buffer size = workers * 10
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
		store:       store,
		cleanupTick: time.NewTicker(cleanupInterval),
	}
	
	// Start workers
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	
	// Start cleanup routine
	p.wg.Add(1)
	go p.cleanupRoutine()
	
	return p
}

// Submit submits a job for async processing
func (p *AsyncProcessor) Submit(job *AsyncPublishJob) error {
	select {
	case p.jobs <- job:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("processor is shutting down")
	default:
		return fmt.Errorf("job queue is full")
	}
}

// Shutdown gracefully shuts down the processor
func (p *AsyncProcessor) Shutdown() {
	slog.Info("Shutting down async processor")
	
	// Stop accepting new jobs
	p.cancel()
	
	// Close the jobs channel
	close(p.jobs)
	
	// Stop cleanup ticker
	p.cleanupTick.Stop()
	
	// Wait for all workers to finish
	p.wg.Wait()
	
	slog.Info("Async processor shutdown complete")
}

// worker processes jobs from the queue
func (p *AsyncProcessor) worker(id int) {
	defer p.wg.Done()
	
	slog.Debug("Async worker started", "worker_id", id)
	
	for job := range p.jobs {
		select {
		case <-p.ctx.Done():
			slog.Debug("Worker stopping due to shutdown", "worker_id", id)
			return
		default:
			p.processJob(job)
		}
	}
	
	slog.Debug("Async worker stopped", "worker_id", id)
}

// processJob processes a single async job
func (p *AsyncProcessor) processJob(job *AsyncPublishJob) {
	slog.Info("Processing async publish job",
		"operation_id", job.Operation.ID,
		"scope", job.Element.Scope,
		"package", job.Element.Name,
		"version", job.Element.Version)
	
	// Simulate some processing time (in real implementation, this would be actual work)
	// For now, we'll just extract manifest files which is already done synchronously
	
	// Check if package already exists
	if job.Repo.Exists(job.Element) {
		p.markFailed(job, "package_exists", fmt.Sprintf("Package already exists: %s", job.Element.FileName()))
		return
	}
	
	// In a real implementation, we might do additional processing here:
	// - Security scanning
	// - Dependency resolution
	// - License validation
	// - Package signing verification
	
	// For now, we'll just mark as successful since the actual file operations
	// were already done in the publish handler
	
	// Build the location URL
	location, err := url.JoinPath(
		utils.BaseUrl(job.Config),
		job.Element.Scope,
		job.Element.Name,
		job.Element.FileName())
	if err != nil {
		p.markFailed(job, "internal_error", fmt.Sprintf("Failed to build location URL: %v", err))
		return
	}
	
	// Mark as completed
	p.markCompleted(job, location)
}

// markCompleted marks an operation as completed successfully
func (p *AsyncProcessor) markCompleted(job *AsyncPublishJob, location string) {
	now := time.Now()
	job.Operation.Status = models.OperationStatusCompleted
	job.Operation.CompletedAt = &now
	job.Operation.Result = &models.OperationResult{
		Location: location,
		Message:  "Package published successfully",
	}
	
	if err := p.store.Update(job.Operation); err != nil {
		slog.Error("Failed to update operation status",
			"operation_id", job.Operation.ID,
			"error", err)
	}
	
	slog.Info("Async operation completed successfully",
		"operation_id", job.Operation.ID,
		"location", location)
}

// markFailed marks an operation as failed
func (p *AsyncProcessor) markFailed(job *AsyncPublishJob, code, message string) {
	now := time.Now()
	job.Operation.Status = models.OperationStatusFailed
	job.Operation.CompletedAt = &now
	job.Operation.Error = &models.OperationError{
		Code:    code,
		Message: message,
	}
	
	if err := p.store.Update(job.Operation); err != nil {
		slog.Error("Failed to update operation status",
			"operation_id", job.Operation.ID,
			"error", err)
	}
	
	slog.Error("Async operation failed",
		"operation_id", job.Operation.ID,
		"code", code,
		"message", message)
}

// cleanupRoutine periodically cleans up old operations
func (p *AsyncProcessor) cleanupRoutine() {
	defer p.wg.Done()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.cleanupTick.C:
			// Clean up operations older than 24 hours
			if err := p.store.DeleteExpired(24 * time.Hour); err != nil {
				slog.Error("Failed to clean up expired operations", "error", err)
			}
		}
	}
}