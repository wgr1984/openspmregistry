package files

import (
	"OpenSPMRegistry/models"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type access struct {
	path     string
	osModule OsAdapter
}

func (f *access) GetWriter(element *models.UploadElement) (io.WriteCloser, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	_, err := f.osModule.Stat(pathFolder)
	if errors.Is(err, os.ErrNotExist) {
		if err := f.osModule.MkdirAll(pathFolder, os.ModePerm); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	pathFile := filepath.Join(pathFolder, element.FileName())

	return f.osModule.Create(pathFile)
}

func (f *access) GetReader(element *models.UploadElement) (io.ReadSeekCloser, error) {
	if !f.Exists(element) {
		return nil, errors.New(fmt.Sprintf("file not exists: %s", element.FileName()))
	}

	pathFile := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	file, err := f.osModule.Open(pathFile)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (f *access) Exists(element *models.UploadElement) bool {
	path := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	if _, err := f.osModule.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}
