package collectionsign

import (
	"testing"

	"OpenSPMRegistry/config"
)

func TestLoadSignerForPackageCollections_Disabled(t *testing.T) {
	s, err := LoadSignerForPackageCollections(config.PackageCollectionsConfig{
		Enabled: true,
		Signing: config.PackageCollectionsSigningConfig{Enabled: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s != nil {
		t.Fatal("expected nil signer")
	}
}

func TestLoadSignerForPackageCollections_SigningWithoutCollections(t *testing.T) {
	_, err := LoadSignerForPackageCollections(config.PackageCollectionsConfig{
		Enabled: false,
		Signing: config.PackageCollectionsSigningConfig{Enabled: true},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadSignerForPackageCollections_MissingPaths(t *testing.T) {
	_, err := LoadSignerForPackageCollections(config.PackageCollectionsConfig{
		Enabled: true,
		Signing: config.PackageCollectionsSigningConfig{
			Enabled: true,
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
