package repo

import (
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

func (f *FileRepo) Exists(scope string, packageName string, version string) bool {
	path := filepath.Join(f.path, scope, packageName, version)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func (f *FileRepo) Write(scope string, packageName string, version string, reader io.Reader) error {
	pathFolder := filepath.Join(f.path, scope, packageName)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(pathFolder, os.ModeDir)
		if err != nil {
			return err
		}
	}
	// write to file
	file, err := os.Create("/tmp/dat2")
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			slog.Error(err.Error())
		}
	}(file)

	b := make([]byte, 1024)
	for {
		count, err := reader.Read(b)
		slog.Debug("read", "count", count)
		if err == io.EOF {
			slog.Debug("EOF")
			break
		}
		_, writeErr := file.Write(b[:count])
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}
