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
	path := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (f *FileRepo) Write(element *models.Element, reader io.Reader) error {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
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
		if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
			return fileCloseErr
		}
		return err
	}

	b := make([]byte, 512)
	for {
		count, err := reader.Read(b)
		slog.Debug("Filerepo read", "count", count)
		wrote, writeErr := file.Write(b[:count])
		if writeErr != nil {
			if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
				return fileCloseErr
			}
			return writeErr
		}
		slog.Debug("Filerepo wrote", "count", wrote)
		if err == io.EOF {
			slog.Debug("filerepo EOF", "filename", pathFile)
			break
		} else if err != nil {
			if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
				return fileCloseErr
			}
			return err
		}
	}

	if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
		return fileCloseErr
	}

	return ExtractPackageSwiftFiles(element, pathFile, writePackageSwiftFiles(pathFolder))
}

func writePackageSwiftFiles(pathFolder string) func(name string, r io.ReadCloser) error {
	return func(name string, r io.ReadCloser) error {
		// write to file
		pathFile := filepath.Join(pathFolder, name)
		file, err := os.Create(pathFile)
		if err != nil {
			if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
				return fileCloseErr
			}
			return err
		}

		b := make([]byte, 512)
		for {
			count, err := r.Read(b)
			slog.Debug("Filerepo read", "filename", pathFile)
			wrote, writeErr := file.Write(b[:count])
			if writeErr != nil {
				if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
					return fileCloseErr
				}
				return writeErr
			}
			slog.Debug("Filerepo wrote", "filename", pathFile, "count", wrote)
			if err == io.EOF {
				slog.Debug("filerepo EOF", "filename", pathFile)
				break
			} else if err != nil {
				if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
					return fileCloseErr
				}
				return err
			}
		}
		return closeFile(pathFile, file)
	}
}

func closeFile(name string, file *os.File) error {
	if file != nil {
		if err := file.Close(); err != nil {
			return err
		}
	}
	slog.Info("Filerepo closed", "filename", name)
	return nil
}
