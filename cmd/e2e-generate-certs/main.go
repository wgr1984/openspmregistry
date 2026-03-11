// e2e-generate-certs generates self-signed TLS certificates for E2E HTTPS testing.
// Cert includes SAN for 127.0.0.1, ::1, and localhost. Output: testdata/e2e/certs/server.crt and server.key.
//
// Run from repo root. Override output dir with E2E_CERTS_DIR.
//
// Usage: go run ./cmd/e2e-generate-certs
package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"OpenSPMRegistry/internal/e2ecerts"
)

func main() {
	certsDir := os.Getenv("E2E_CERTS_DIR")
	if certsDir == "" {
		cwd, _ := os.Getwd()
		certsDir = filepath.Join(cwd, "testdata", "e2e", "certs")
	}
	if err := e2ecerts.Generate(certsDir); err != nil {
		slog.Error("generate certs", "err", err)
		os.Exit(1)
	}
	slog.Info("generated certs", "dir", certsDir)
}
