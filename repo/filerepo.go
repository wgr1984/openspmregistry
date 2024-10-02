package repo

import (
	"OpenSPMRegistry/models"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type FileRepo struct {
	path string
}

func NewFileRepo(path string) *FileRepo {
	return &FileRepo{path: path}
}

func (f *FileRepo) Exists(element *models.Element) bool {
	path := filepath.Join(f.path, element.Scope, element.Name, element.FileName())
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (f *FileRepo) Write(element *models.Element, reader io.Reader) error {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(pathFolder, os.ModePerm)
		if err != nil {
			return err
		}
	}

	// write to file
	pathFile := filepath.Join(pathFolder, element.FileName())
	file, err := os.Create(pathFile)
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			slog.Error(err.Error())
		}
		slog.Info("filerepo file closed")
	}(file)

	b := make([]byte, 512)
	for {
		count, err := reader.Read(b)
		slog.Debug("Filerepo read", "count", count)
		if err == io.EOF {
			slog.Debug("Filerepo EOF")
			break
		}
		_, writeErr := file.Write(b[:count])
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}
