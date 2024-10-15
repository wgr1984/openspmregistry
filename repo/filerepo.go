package repo

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	if element.MimeType == mimetypes.ApplicationZip {
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

func (f *FileRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		return time.Now(), errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}
	pathFile := filepath.Join(pathFolder, element.FileName())
	stat, err := os.Stat(pathFile)
	if err != nil {
		return time.Now(), err
	}

	return stat.ModTime(), nil
}

func (f *FileRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
	pathFolder := filepath.Join(f.path, scope, name, version)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}

	metadata := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.Metadata)
	if !f.Exists(metadata) {
		return nil, errors.New(fmt.Sprintf("file not exists: %s", metadata.FileName()))
	}

	var b bytes.Buffer
	writer := bufio.NewWriter(&b)
	if err := f.Read(metadata, writer); err != nil {
		return nil, err
	}
	if err := writer.Flush(); err != nil {
		return nil, err
	}

	var metadataResult map[string]interface{}
	if err := json.Unmarshal(b.Bytes(), &metadataResult); err != nil {
		return nil, err
	}

	return metadataResult, nil
}

func (f *FileRepo) Checksum(element *models.UploadElement) (string, error) {
	if !f.Exists(element) {
		return "", errors.New(fmt.Sprintf("file not exists: %s", element.FileName()))
	}

	pathFile := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	file, err := os.Open(pathFile)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (f *FileRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	if _, err := os.Stat(pathFolder); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}

	var manifests []models.UploadElement
	// search for different versions of Package.swift manifest

	// search for different versions of Package.swift manifest
	err := filepath.WalkDir(pathFolder, func(p string, d os.DirEntry, err error) error {
		// skip root
		if p == pathFolder {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		filename := d.Name()
		if filename != "Package.swift" && strings.HasPrefix(filename, "Package") && filepath.Ext(p) == ".swift" {
			manifest := models.NewUploadElement(element.Scope, element.Name, element.Version, mimetypes.TextXSwift, models.Manifest)
			manifest.SetFilenameOverwrite(strings.TrimSuffix(filename, filepath.Ext(filename)))
			manifests = append(manifests, *manifest)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return manifests, nil
}

func (f *FileRepo) GetSwiftToolVersion(manifest *models.UploadElement) (string, error) {
	if !f.Exists(manifest) {
		return "", errors.New(fmt.Sprintf("file not exists: %s", manifest.FileName()))
	}

	pathFile := filepath.Join(f.path, manifest.Scope, manifest.Name, manifest.Version, manifest.FileName())
	file, err := os.Open(pathFile)
	if err != nil {
		return "", err
	}

	const swiftVersionPrefix = "// swift-tools-version:"
	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, swiftVersionPrefix) {
			return strings.TrimPrefix(line, swiftVersionPrefix), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("swift-tools-version not found")
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
