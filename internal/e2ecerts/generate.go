// Package e2ecerts provides E2E test TLS certificate generation (self-signed, SAN for localhost/127.0.0.1).
// Used by cmd/e2e-generate-certs and e2e tests so certs can be generated in-process without exec.
package e2ecerts

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	keySize      = 2048
	validityDays = 365
	cn           = "OpenSPMRegistry E2E Test"
)

// Generate creates server.crt and server.key in certsDir (created if missing).
// Cert has SAN for 127.0.0.1, ::1, and localhost. Idempotent for same dir; overwrites existing files.
//
// Parameters:
//   - certsDir: directory to write server.crt and server.key (e.g. testdata/e2e/certs)
//
// Returns:
//   - error: non-nil if directory creation or cert write fails
func Generate(certsDir string) error {
	if err := os.MkdirAll(certsDir, 0o755); err != nil {
		return err
	}
	keyPath := filepath.Join(certsDir, "server.key")
	crtPath := filepath.Join(certsDir, "server.crt")

	key, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, nil)
	if err != nil {
		return err
	}
	notBefore := time.Now()
	notAfter := notBefore.Add(validityDays * 24 * time.Hour)
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"127.0.0.1", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writeKey(keyPath, key); err != nil {
		return err
	}
	return writeCert(crtPath, der)
}

func writeKey(path string, key *rsa.PrivateKey) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func writeCert(path string, der []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}
