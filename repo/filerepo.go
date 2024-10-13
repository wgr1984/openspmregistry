package repo

import (
	"OpenSPMRegistry/models"
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type FileRepo struct {
	path string
}

func NewFileRepo(path string) *FileRepo {
	return &FileRepo{path: path}
}

func (f *FileRepo) Exists(element *models.UploadElement) bool {
	path := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (f *FileRepo) Write(element *models.UploadElement, reader io.Reader) error {
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

	if element.MimeType == "application/zip" {
		return ExtractPackageSwiftFiles(element, pathFile, writePackageSwiftFiles(pathFolder))
	}

	return nil
}

func (f *FileRepo) Read(element *models.UploadElement, writer io.Writer) error {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		return errors.New(fmt.Sprintf("file not exists: %s", pathFolder))
	}

	// read file
	pathFile := filepath.Join(pathFolder, element.FileName())
	file, err := os.Open(pathFile)
	if err != nil {
		if fileCloseErr := closeFile(pathFile, file); fileCloseErr != nil {
			return fileCloseErr
		}
		return err
	}

	b := make([]byte, 512)
	for {
		count, err := file.Read(b)
		slog.Debug("Filerepo read", "count", count)
		wrote, writeErr := writer.Write(b[:count])
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

	return nil
}

func (f *FileRepo) List(scope string, name string) ([]models.ListElement, error) {
	path := filepath.Join(f.path, scope, name)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	var elements []models.ListElement

	err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		// skip root
		if p == path {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			elements = append(elements, *models.NewListElement(scope, name, d.Name()))
			return nil
		}
		return nil
	})

	return elements, err
}

func (f *FileRepo) EncodeBase64(element *models.UploadElement) (string, error) {
	if !f.Exists(element) {
		return "", errors.New(fmt.Sprintf("file not exists: %s", element.FileName()))
	}

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)

	if err := f.Read(element, writer); err != nil {
		return "", err
	}

	if err := writer.Flush(); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

func (f *FileRepo) PublishDate(element *models.UploadElement) (*time.Time, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}
	pathFile := filepath.Join(pathFolder, element.FileName())
	stat, err := os.Stat(pathFile)
	if err != nil {
		return nil, err
	}
	modTime := stat.ModTime()
	return &modTime, nil
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
