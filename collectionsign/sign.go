// Package collectionsign produces SwiftPM-compatible signed package collections
// (JWS over the collection JSON plus root-level signature metadata), matching
// PackageCollectionsSigning in swift-package-manager.
package collectionsign

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"

	"OpenSPMRegistry/models"
)

// Signer signs package collections using a certificate chain and private key.
type Signer struct {
	certChain [][]byte
	leaf      *x509.Certificate
	key       crypto.Signer
	alg       string // ES256 or RS256
}

type jwsHeader struct {
	Algorithm string   `json:"alg"`
	CertChain []string `json:"x5c"`
}

// signedCollectionSignature is the JSON object under the root "signature" key (Swift PackageCollectionModel.V1.Signature).
type signedCollectionSignature struct {
	Signature   string        `json:"signature"`
	Certificate signatureCert `json:"certificate"`
}

type signatureCert struct {
	Subject certName `json:"subject"`
	Issuer  certName `json:"issuer"`
}

type certName struct {
	UserID             *string `json:"userID,omitempty"`
	CommonName         *string `json:"commonName,omitempty"`
	OrganizationalUnit *string `json:"organizationalUnit,omitempty"`
	Organization       *string `json:"organization,omitempty"`
}

const minRSABits = 2048

// NewSignerFromFiles loads DER certificates (leaf first, then intermediates) and a PEM private key.
func NewSignerFromFiles(certChainPaths []string, privateKeyPath string) (*Signer, error) {
	if len(certChainPaths) == 0 {
		return nil, errors.New("collection signing: empty certificate chain")
	}
	chain := make([][]byte, 0, len(certChainPaths))
	for _, p := range certChainPaths {
		der, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("collection signing: read cert %q: %w", p, err)
		}
		chain = append(chain, der)
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, fmt.Errorf("collection signing: parse leaf certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("collection signing: read private key %q: %w", privateKeyPath, err)
	}
	key, err := parsePrivateKeyPEM(keyPEM)
	if err != nil {
		return nil, err
	}

	var alg string
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		if k.Curve.Params().Name != "P-256" {
			return nil, errors.New("collection signing: only P-256 (ES256) ECDSA keys are supported")
		}
		alg = "ES256"
	case *rsa.PrivateKey:
		if k.N.BitLen() < minRSABits {
			return nil, fmt.Errorf("collection signing: RSA key must be at least %d bits", minRSABits)
		}
		alg = "RS256"
	default:
		return nil, errors.New("collection signing: unsupported private key type")
	}

	return &Signer{
		certChain: chain,
		leaf:      leaf,
		key:       key,
		alg:       alg,
	}, nil
}

func parsePrivateKeyPEM(pemData []byte) (crypto.Signer, error) {
	var block *pem.Block
	for {
		block, pemData = pem.Decode(pemData)
		if block == nil {
			return nil, errors.New("collection signing: no PEM block in private key file")
		}
		if block.Type == "EC PRIVATE KEY" || block.Type == "RSA PRIVATE KEY" || block.Type == "PRIVATE KEY" {
			break
		}
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if s, ok := key.(crypto.Signer); ok {
			return s, nil
		}
		return nil, errors.New("collection signing: PKCS#8 key is not a signer")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("collection signing: could not parse private key PEM")
}

// WrapSignedCollection returns JSON bytes: all collection fields plus "signature", compatible with SwiftPM SignedCollection.
func (s *Signer) WrapSignedCollection(coll *models.PackageCollection) ([]byte, error) {
	payload, err := json.Marshal(coll)
	if err != nil {
		return nil, fmt.Errorf("collection signing: marshal collection: %w", err)
	}

	x5c := make([]string, len(s.certChain))
	for i, der := range s.certChain {
		x5c[i] = base64.StdEncoding.EncodeToString(der)
	}
	header := jwsHeader{Algorithm: s.alg, CertChain: x5c}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("collection signing: marshal JWS header: %w", err)
	}

	encHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encPayload := base64.RawURLEncoding.EncodeToString(payload)
	signingInput := encHeader + "." + encPayload

	sum := sha256.Sum256([]byte(signingInput))
	sigBytes, err := signDigest(s.key, s.alg, sum[:])
	if err != nil {
		return nil, err
	}
	encSig := base64.RawURLEncoding.EncodeToString(sigBytes)
	jws := encHeader + "." + encPayload + "." + encSig

	sigObj := signedCollectionSignature{
		Signature: jws,
		Certificate: signatureCert{
			Subject: certNameFromPKIX(s.leaf.Subject),
			Issuer:  certNameFromPKIX(s.leaf.Issuer),
		},
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, fmt.Errorf("collection signing: payload to map: %w", err)
	}
	sigField, err := json.Marshal(sigObj)
	if err != nil {
		return nil, fmt.Errorf("collection signing: marshal signature object: %w", err)
	}
	root["signature"] = sigField
	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("collection signing: marshal signed collection: %w", err)
	}
	return out, nil
}

func certNameFromPKIX(n pkix.Name) certName {
	out := certName{}
	if n.CommonName != "" {
		s := n.CommonName
		out.CommonName = &s
	}
	if len(n.Organization) > 0 {
		s := n.Organization[0]
		out.Organization = &s
	}
	if len(n.OrganizationalUnit) > 0 {
		s := n.OrganizationalUnit[0]
		out.OrganizationalUnit = &s
	}
	return out
}

func signDigest(key crypto.Signer, alg string, digest []byte) ([]byte, error) {
	switch alg {
	case "ES256":
		ec, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("collection signing: internal error: ES256 without ECDSA key")
		}
		r, s, err := ecdsa.Sign(rand.Reader, ec, digest)
		if err != nil {
			return nil, fmt.Errorf("collection signing: ECDSA sign: %w", err)
		}
		return encodeECDSAP256Raw(r, s), nil
	case "RS256":
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("collection signing: internal error: RS256 without RSA key")
		}
		return rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest)
	default:
		return nil, fmt.Errorf("collection signing: unknown algorithm %q", alg)
	}
}

// encodeECDSAP256Raw returns R||S as 32-byte big-endian integers (Swift Crypto P256.Signature rawRepresentation).
func encodeECDSAP256Raw(r, s *big.Int) []byte {
	rb := r.Bytes()
	sb := s.Bytes()
	out := make([]byte, 64)
	copy(out[32-len(rb):32], rb)
	copy(out[64-len(sb):], sb)
	return out
}
