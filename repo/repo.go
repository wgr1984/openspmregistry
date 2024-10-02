package repo

import (
	"OpenSPMRegistry/models"
	"io"
)

type Repo interface {
	Exists(element *models.Element) bool
	Write(element *models.Element, reader io.Reader) error
}
