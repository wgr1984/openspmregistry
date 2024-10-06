package repo

import (
	"OpenSPMRegistry/models"
	"io"
)

type Repo interface {
	// Exists checks whether element to be published exists already
	// Returns true in case it does otherwise false
	Exists(element *models.UploadElement) bool

	// Write writes the element uploaded into the repo using
	// the reader to gain access to the uploaded data
	// Returns error in case something went wrong
	Write(element *models.UploadElement, reader io.Reader) error

	// List all versions of a certain package existing
	// - `scope` of the package
	// - `name` of the package
	// returns (releases found|nil if not exists, error)
	List(scope string, name string) ([]models.ListElement, error)
}
