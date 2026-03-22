package collectionsign

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"OpenSPMRegistry/models"
)

func TestNewSignerFromFiles_ECDSA(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestP256Chain(t, dir)

	s, err := NewSignerFromFiles([]string{certPath}, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.alg != "ES256" {
		t.Fatalf("alg: got %q", s.alg)
	}
}

func TestNewSignerFromFiles_Errors(t *testing.T) {
	if _, err := NewSignerFromFiles(nil, "k.pem"); err == nil {
		t.Fatal("expected error for empty chain")
	}
	dir := t.TempDir()
	if _, err := NewSignerFromFiles([]string{filepath.Join(dir, "missing.der")}, "k.pem"); err == nil {
		t.Fatal("expected error for missing cert")
	}
}

func TestWrapSignedCollection_VerifyJWS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeTestP256Chain(t, dir)
	s, err := NewSignerFromFiles([]string{certPath}, keyPath)
	if err != nil {
		t.Fatal(err)
	}

	coll := &models.PackageCollection{
		Name:          "Test",
		Overview:      "Overview",
		FormatVersion: "1.0",
		Revision:      1,
		GeneratedAt:   "2025-01-01T00:00:00Z",
		GeneratedBy:   models.GeneratedBy{Name: "OpenSPMRegistry"},
		Packages: []models.CollectionPackage{
			{
				URL: "scope.pkg",
				Versions: []models.PackageVersion{
					{
						Version:             "1.0.0",
						DefaultToolsVersion: "5.10",
						Manifests: map[string]models.PackageManifest{
							"5.10": {
								ToolsVersion: "5.10",
								PackageName:  "pkg",
								Targets:      []models.Target{{Name: "T"}},
								Products:     []models.Product{{Name: "P", Targets: []string{"T"}}},
							},
						},
					},
				},
			},
		},
	}

	signed, err := s.WrapSignedCollection(coll)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(signed, &root); err != nil {
		t.Fatal(err)
	}
	sigRaw, ok := root["signature"]
	if !ok {
		t.Fatal("missing signature key")
	}
	var sigObj signedCollectionSignature
	if err := json.Unmarshal(sigRaw, &sigObj); err != nil {
		t.Fatal(err)
	}
	parts := splitJWS(sigObj.Signature)
	if len(parts) != 3 {
		t.Fatalf("JWS parts: got %d", len(parts))
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var hdr jwsHeader
	if err := json.Unmarshal(headerJSON, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr.Algorithm != "ES256" {
		t.Fatalf("header alg: %q", hdr.Algorithm)
	}
	if len(hdr.CertChain) < 1 {
		t.Fatal("empty x5c")
	}
	certDER, err := base64.StdEncoding.DecodeString(hdr.CertChain[0])
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}
	pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("expected ECDSA public key")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var inner models.PackageCollection
	if err := json.Unmarshal(payload, &inner); err != nil {
		t.Fatal(err)
	}
	if inner.Name != coll.Name || len(inner.Packages) != len(coll.Packages) {
		t.Fatal("inner payload collection mismatch")
	}

	msg := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(msg))
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(sigBytes) != 64 {
		t.Fatalf("ECDSA raw sig len %d", len(sigBytes))
	}
	r := new(big.Int).SetBytes(sigBytes[:32])
	sBig := new(big.Int).SetBytes(sigBytes[32:])
	if !ecdsa.Verify(pub, digest[:], r, sBig) {
		t.Fatal("ECDSA verify failed")
	}
}

func splitJWS(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func writeTestP256Chain(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "collection-signer",
			Organization:       []string{"Test Org"},
			OrganizationalUnit: []string{"Test OU"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(dir, "leaf.der")
	if err := os.WriteFile(certPath, der, 0o600); err != nil {
		t.Fatal(err)
	}
	pk8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	pemBuf := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk8})
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(keyPath, pemBuf, 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
