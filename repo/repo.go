package repo

import (
	"OpenSPMRegistry/models"
	"io"
	"time"
)

// Repo is the interface that wraps the basic operations
// that a repository should implement
type Repo interface {
	// Exists checks whether element to be published exists already
	// Returns true in case it does otherwise false
	Exists(element *models.UploadElement) bool

	// GetReader returns a reader for the specified in the element
	// returns (reader for the file|error)
	GetReader(element *models.UploadElement) (io.ReadSeekCloser, error)

	// GetWriter returns a writer for the specified element
	// returns (writer for the file|error
	GetWriter(element *models.UploadElement) (io.WriteCloser, error)

	// ExtractManifestFiles extracts the manifest files from the provided
	// source archive
	ExtractManifestFiles(element *models.UploadElement) error

	// List all versions of a certain package existing
	// - `scope` of the package
	// - `name` of the package
	// returns (releases found|nil if not exists, error)
	List(scope string, name string) ([]models.ListElement, error)

	// EncodeBase64 returns the base64 representation of the content
	// of the provided element
	// returns (base64 string of content|nil if not exists, error)
	EncodeBase64(element *models.UploadElement) (string, error)

	// PublishDate returns the date element was upload / published
	PublishDate(element *models.UploadElement) (time.Time, error)

	// Checksum provides the sha256 checksum of the element
	// returns (checksum string|empty string if not exists, error)
	Checksum(element *models.UploadElement) (string, error)

	// FetchMetadata retrieves the metadata of the package
	// - `scope` of the package
	// - `name` of the package
	// - `version` of the package
	// returns (metadata map|nil if not exists, error)
	FetchMetadata(scope string, name string, version string) (map[string]interface{}, error)

	// GetAlternativeManifests returns the alternative versions of the manifest
	// returns (alternative versions of the manifest|nil if not exists, error)
	GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error)

	// GetSwiftToolVersion returns the swift tool version
	// specified in the first line of the manifest file
	// returns (swift tool version|nil if not exists, error)
	GetSwiftToolVersion(manifest *models.UploadElement) (string, error)
}
