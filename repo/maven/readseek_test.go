package maven

import (
	"OpenSPMRegistry/config"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func Test_newRangeReadSeekCloser_ValidResponse_ReturnsReader(t *testing.T) {
	testData := make([]byte, 100)
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "bytes=0-" {
				w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(testData)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	if reader == nil {
		t.Errorf("expected reader, got nil")
		return
	}
	if reader.size != int64(dataLen) {
		t.Errorf("expected size %d, got %d", dataLen, reader.size)
	}
}

func Test_newRangeReadSeekCloser_NoContentLength_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	_, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err == nil {
		t.Errorf("expected error for missing Content-Length, got nil")
	}
}

func Test_rangeReadSeekCloser_Read_ReadsData(t *testing.T) {
	testData := []byte("test data for reading")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "bytes=0-" {
				w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(testData)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, len(testData))
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("expected to read %d bytes, read %d", len(testData), n)
	}
	if string(buf) != string(testData) {
		t.Errorf("expected '%s', got '%s'", string(testData), string(buf))
	}
}

func Test_rangeReadSeekCloser_Seek_SeeksToPosition(t *testing.T) {
	testData := []byte("0123456789abcdefghij")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			rangeHeader := r.Header.Get("Range")
			if rangeHeader == "bytes=10-" {
				w.Header().Set("Content-Range", "bytes 10-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(testData[10:])
			} else if rangeHeader == "bytes=0-" {
				w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(testData)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	// Seek to position 10
	pos, err := reader.Seek(10, io.SeekStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 10 {
		t.Errorf("expected position 10, got %d", pos)
	}

	// Read from position 10
	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := string(testData[10:])
	if string(buf[:n]) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(buf[:n]))
	}
}

func Test_rangeReadSeekCloser_Seek_SeekStart(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	pos, err := reader.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 5 {
		t.Errorf("expected position 5, got %d", pos)
	}
}

func Test_rangeReadSeekCloser_Seek_SeekCurrent(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	// Read some data first
	buf := make([]byte, 3)
	if _, err := reader.Read(buf); err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}

	// Seek relative to current position
	pos, err := reader.Seek(2, io.SeekCurrent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 5 {
		t.Errorf("expected position 5, got %d", pos)
	}
}

func Test_rangeReadSeekCloser_Seek_SeekEnd(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	pos, err := reader.Seek(-3, io.SeekEnd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := int64(len(testData)) - 3
	if pos != expected {
		t.Errorf("expected position %d, got %d", expected, pos)
	}
}

func Test_rangeReadSeekCloser_Seek_NegativePosition_ReturnsError(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	_, err = reader.Seek(-1, io.SeekStart)
	if err == nil {
		t.Errorf("expected error for negative position, got nil")
	}
}

func Test_rangeReadSeekCloser_Seek_BeyondSize_ClampsToSize(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	pos, err := reader.Seek(100, io.SeekStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != int64(len(testData)) {
		t.Errorf("expected position clamped to %d, got %d", len(testData), pos)
	}
}

func Test_rangeReadSeekCloser_Close_ClosesReader(t *testing.T) {
	testData := []byte("test data")
	dataLen := len(testData)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusOK)
		} else if r.Method == "GET" {
			w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(dataLen-1)+"/"+strconv.Itoa(dataLen))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newRangeReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = reader.Close()
	if err != nil {
		t.Errorf("unexpected error closing: %v", err)
	}

	// Reading after close should fail
	_, err = reader.Read(make([]byte, 1))
	if err == nil {
		t.Errorf("expected error reading after close, got nil")
	}
}

func Test_newBufferedReadSeekCloser_ValidResponse_ReturnsReader(t *testing.T) {
	testData := []byte("test data for buffering")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newBufferedReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != string(testData) {
		t.Errorf("expected '%s', got '%s'", string(testData), string(data))
	}
}

func Test_bufferedReadSeekCloser_Seek_SeeksToPosition(t *testing.T) {
	testData := []byte("0123456789")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newBufferedReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	// Seek to position 5
	pos, err := reader.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos != 5 {
		t.Errorf("expected position 5, got %d", pos)
	}

	// Read from position 5
	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := string(testData[5:])
	if string(buf[:n]) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(buf[:n]))
	}
}

func Test_bufferedReadSeekCloser_Close_ClosesReader(t *testing.T) {
	testData := []byte("test data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newBufferedReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = reader.Close()
	if err != nil {
		t.Errorf("unexpected error closing: %v", err)
	}

	// Multiple closes should be safe
	err = reader.Close()
	if err != nil {
		t.Errorf("unexpected error on second close: %v", err)
	}
}

func Test_bufferedReadSeekCloser_ReadAll_ReadsAllData(t *testing.T) {
	testData := bytes.Repeat([]byte("a"), 1000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(testData)
		}
	}))
	defer server.Close()

	cfg := config.MavenConfig{BaseURL: server.URL}
	c, _ := newClient(cfg)

	reader, err := newBufferedReadSeekCloser(c, "test/path", context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) != len(testData) {
		t.Errorf("expected %d bytes, got %d", len(testData), len(data))
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("data mismatch")
	}
}
