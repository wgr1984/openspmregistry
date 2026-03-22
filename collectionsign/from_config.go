package collectionsign

import (
	"errors"
	"fmt"

	"OpenSPMRegistry/config"
)

// LoadSignerForPackageCollections returns a Signer when package collections and signing are enabled in config.
// It returns (nil, nil) when signing is disabled. It returns an error if signing is enabled but misconfigured
// or if loading keys/certs fails.
func LoadSignerForPackageCollections(pc config.PackageCollectionsConfig) (*Signer, error) {
	if !pc.Signing.Enabled {
		return nil, nil
	}
	if !pc.Enabled {
		return nil, errors.New("packageCollections.signing.enabled requires packageCollections.enabled")
	}
	if len(pc.Signing.CertChain) == 0 || pc.Signing.PrivateKey == "" {
		return nil, errors.New("packageCollections.signing.enabled requires certChain (non-empty) and privateKey")
	}
	s, err := NewSignerFromFiles(pc.Signing.CertChain, pc.Signing.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("package collection signing: %w", err)
	}
	return s, nil
}
