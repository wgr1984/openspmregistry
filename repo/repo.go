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

	// Write writes the element uploaded into the repo using
	// the reader to gain access to the uploaded data
	// Returns error in case something went wrong
	Write(element *models.UploadElement, reader io.Reader) error

	// Read reads the element uploaded to the repo using
	// a writer to gain access to the uploaded data
	// Returns error in case something went wrong
	Read(element *models.UploadElement, writer io.Writer) error

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
}
