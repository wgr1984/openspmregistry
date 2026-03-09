package maven

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
)

// rangeReadSeekCloser implements io.ReadSeekCloser using HTTP Range requests
type rangeReadSeekCloser struct {
	client *client
	url    string
	ctx    context.Context
	pos    int64
	size   int64
	mu     sync.Mutex
	body   io.ReadCloser
	closed bool
}

// bufferedReadSeekCloser implements io.ReadSeekCloser by buffering the entire response
type bufferedReadSeekCloser struct {
	*bytes.Reader
	closed bool
}

// newRangeReadSeekCloser creates a new range-based ReadSeekCloser
func newRangeReadSeekCloser(ctx context.Context, client *client, url string) (*rangeReadSeekCloser, error) {
	// First, get the size via HEAD request
	resp, err := client.HEAD(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get size: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	size := resp.ContentLength
	if size < 0 {
		// If Content-Length is not available, we can't use range requests effectively
		// Fall back to buffering
		return nil, fmt.Errorf("Content-Length not available")
	}

	r := &rangeReadSeekCloser{
		client: client,
		url:    url,
		ctx:    ctx,
		pos:    0,
		size:   size,
	}

	// Fetch initial data (lock not needed here since no other goroutine can access r yet)
	if err := r.fetchRange(0, -1); err != nil {
		return nil, err
	}

	return r, nil
}

// fetchRange fetches a byte range from the server
// Caller must hold r.mu lock
func (r *rangeReadSeekCloser) fetchRange(start int64, end int64) error {
	if r.closed {
		return io.ErrClosedPipe
	}

	// Close existing body if any
	if r.body != nil {
		_ = r.body.Close()
		r.body = nil
	}

	req, err := r.client.makeRequest(r.ctx, "GET", r.url, nil)
	if err != nil {
		return err
	}

	// Set Range header
	if end < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	}

	resp, err := r.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("range request failed: %w", err)
	}

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	r.body = resp.Body
	return nil
}

// Read reads data from the current position
func (r *rangeReadSeekCloser) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.ErrClosedPipe
	}

	if r.body == nil {
		// Need to fetch data at current position
		if err := r.fetchRange(r.pos, -1); err != nil {
			return 0, err
		}
	}

	n, err = r.body.Read(p)
	r.pos += int64(n)

	// If we hit EOF, try to fetch more if we haven't reached the end
	if err == io.EOF && r.pos < r.size {
		// Close current body and fetch next range
		_ = r.body.Close()
		r.body = nil
		// Try to read more
		if err2 := r.fetchRange(r.pos, -1); err2 == nil {
			// Continue reading
			n2, err2 := r.body.Read(p[n:])
			n += n2
			err = err2
			r.pos += int64(n2)
		}
	}

	return n, err
}

// Seek seeks to a specific position
func (r *rangeReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, io.ErrClosedPipe
	}

	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = r.pos + offset
	case io.SeekEnd:
		newPos = r.size + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}

	if newPos < 0 {
		return 0, fmt.Errorf("negative position")
	}
	if newPos > r.size {
		newPos = r.size
	}

	r.pos = newPos

	// Close current body and fetch new range
	if r.body != nil {
		_ = r.body.Close()
		r.body = nil
	}

	// Fetch data at new position
	if err := r.fetchRange(r.pos, -1); err != nil {
		return 0, err
	}

	return r.pos, nil
}

// Close closes the reader
func (r *rangeReadSeekCloser) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true
	if r.body != nil {
		return r.body.Close()
	}
	return nil
}

// newBufferedReadSeekCloser creates a new buffered ReadSeekCloser
func newBufferedReadSeekCloser(ctx context.Context, client *client, url string) (*bufferedReadSeekCloser, error) {
	resp, err := client.GET(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		slog.Debug("Buffered HTTP response", "url", url, "size", len(data))
	}

	return &bufferedReadSeekCloser{
		Reader: bytes.NewReader(data),
		closed: false,
	}, nil
}

// Close closes the reader (releases buffer reference)
func (b *bufferedReadSeekCloser) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	// bytes.Reader doesn't need explicit cleanup, but we mark as closed
	return nil
}
