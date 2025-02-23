package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"OpenSPMRegistry/repo"
	"bufio"
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
	repo.Access
	path     string
	osModule OsAdapter
}

func NewFileRepo(path string) *FileRepo {
	osModule := &osAdapterDefault{}
	return &FileRepo{
		path:     path,
		osModule: osModule,
		Access: &access{
			path:     path,
			osModule: osModule,
		},
	}
}

func (f *FileRepo) ExtractManifestFiles(element *models.UploadElement) error {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	_, err := f.osModule.Stat(pathFolder)
	if errors.Is(err, os.ErrNotExist) {
		err := f.osModule.MkdirAll(pathFolder, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}

	pathFile := filepath.Join(pathFolder, element.FileName())

	if element.MimeType == mimetypes.ApplicationZip {
		return ExtractPackageSwiftFiles(element, pathFile, writePackageSwiftFiles(pathFolder))
	}

	return errors.New("unsupported mime type")
}

func (f *FileRepo) List(scope string, name string) ([]models.ListElement, error) {
	path := filepath.Join(f.path, scope, name)
	_, err := f.osModule.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}

	var elements []models.ListElement

	err = f.osModule.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		// exit on error
		if err != nil {
			return err
		}
		// skip root
		if p == path {
			return nil
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

	reader, err := f.GetReader(element)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := reader.Close(); err != nil {
			slog.Error("Error closing reader:", "error", err)
		}
	}()

	b, err2 := io.ReadAll(reader)
	if err2 != nil {
		return "", err2
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

func (f *FileRepo) PublishDate(element *models.UploadElement) (time.Time, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	_, err := f.osModule.Stat(pathFolder)
	if errors.Is(err, os.ErrNotExist) {
		return time.Now(), errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}
	if err != nil {
		return time.Now(), err
	}
	pathFile := filepath.Join(pathFolder, element.FileName())
	stat, err := f.osModule.Stat(pathFile)
	if err != nil {
		return time.Now(), err
	}

	return stat.ModTime(), nil
}

func (f *FileRepo) FetchMetadata(scope string, name string, version string) (map[string]interface{}, error) {
	pathFolder := filepath.Join(f.path, scope, name, version)
	_, err := f.osModule.Stat(pathFolder)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}
	if err != nil {
		return nil, err
	}

	metadata := models.NewUploadElement(scope, name, version, mimetypes.ApplicationJson, models.Metadata)
	if !f.Exists(metadata) {
		return nil, errors.New(fmt.Sprintf("file not exists: %s", metadata.FileName()))
	}

	reader, err := f.GetReader(metadata)
	if err != nil {
		return nil, err
	}

	defer func() {
		if reader == nil {
			return
		}
		if err := reader.Close(); err != nil {
			slog.Error("Error closing reader:", "error", err)
		}
	}()

	b, err2 := io.ReadAll(reader)
	if err2 != nil {
		return nil, err2
	}

	var metadataResult map[string]interface{}
	if err := json.Unmarshal(b, &metadataResult); err != nil {
		return nil, err
	}

	return metadataResult, nil
}

func (f *FileRepo) Checksum(element *models.UploadElement) (string, error) {
	if !f.Exists(element) {
		return "", errors.New(fmt.Sprintf("file not exists: %s", element.FileName()))
	}

	pathFile := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	file, err := f.osModule.Open(pathFile)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	if err := file.Close(); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (f *FileRepo) GetAlternativeManifests(element *models.UploadElement) ([]models.UploadElement, error) {
	pathFolder := filepath.Join(f.path, element.Scope, element.Name, element.Version)
	_, err := f.osModule.Stat(pathFolder)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(fmt.Sprintf("path does not exists: %s", pathFolder))
	}
	if err != nil {
		return nil, err
	}

	var manifests []models.UploadElement
	// search for different versions of Package.swift manifest

	// search for different versions of Package.swift manifest
	err = f.osModule.WalkDir(pathFolder, func(p string, d os.DirEntry, err error) error {
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
	file, err := f.osModule.Open(pathFile)
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

func (f *FileRepo) Lookup(url string) []string {
	var result []string

	err := filepath.WalkDir(f.path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), "metadata.json") {
			return nil
		}

		version := filepath.Base(filepath.Dir(path))
		scope := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(path))))
		packageName := filepath.Base(filepath.Dir(filepath.Dir(path)))
		metadata, err := f.FetchMetadata(scope, packageName, version)
		if err != nil {
			return nil
		}

		if repositoryURLs, ok := metadata["repositoryURLs"].([]interface{}); ok {
			for _, repoURL := range repositoryURLs {
				if repoURLStr, ok := repoURL.(string); ok && repoURLStr == url {
					foundId := fmt.Sprintf("%s.%s", scope, packageName)
					result = append(result, foundId)
					// take first match found
					return filepath.SkipAll
				}
			}
		}

		return nil
	})

	if err != nil {
		slog.Error("Error walking through directories:", "error", err)
	}

	return result
}

func (f *FileRepo) Remove(element *models.UploadElement) error {
	path := filepath.Join(f.path, element.Scope, element.Name, element.Version, element.FileName())
	if f.Exists(element) {
		return os.Remove(path)
	}
	return errors.New(fmt.Sprintf("file not exists: %s", element.FileName()))
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
