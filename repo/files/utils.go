package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
	"fmt"
	"io"
	"path"
	"strings"
)

// ExtractManifestFilesFromZipReader extracts Package.swift and Package.json files from a zip.Reader
// and calls fileExtractor for each matching file found.
func ExtractManifestFilesFromZipReader(element *models.UploadElement, zipReader *zip.Reader, fileExtractor func(name string, r io.ReadCloser) error) error {
	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		cleanName := path.Clean(file.Name)
		dir := path.Dir(cleanName)
		base := path.Base(cleanName)
		ext := path.Ext(base)
		scope := element.Scope
		name := element.Name
		version := element.Version
		id := fmt.Sprintf("%s.%s", scope, name)

		// Accept manifests at archive root or in a single top-level directory.
		// Official SPM (swift package archive-source / package-registry publish) uses scope.name as top-level dir
		// (e.g. example.UtilsPackage/Package.swift). We also accept root and "Name-version" as fallbacks for
		// other clients or SPM versions; the Registry spec does not mandate zip layout.
		atRoot := dir == "." || dir == ""
		singleTopLevel := dir == id || dir == name+"-"+version
		nestedUnderId := strings.HasPrefix(dir, id) && !strings.Contains(strings.TrimPrefix(strings.ToLower(dir), strings.ToLower(id)), "/")
		if !atRoot && !singleTopLevel && !nestedUnderId {
			continue
		}

		// Extract Package.swift files
		if strings.HasPrefix(strings.ToLower(base), "package") && strings.ToLower(ext) == ".swift" {
			readerCloser, err := file.Open()
			if err != nil {
				return err
			}

			if errReader := fileExtractor(base, readerCloser); errReader != nil {
				if e := ensureReaderClosed(readerCloser); e != nil {
					return e
				}
				return errReader
			}

			if e := ensureReaderClosed(readerCloser); e != nil {
				return e
			}
		}

		// Extract Package.json file
		if strings.ToLower(base) == "package.json" {
			readerCloser, err := file.Open()
			if err != nil {
				return err
			}

			if errReader := fileExtractor(base, readerCloser); errReader != nil {
				if e := ensureReaderClosed(readerCloser); e != nil {
					return e
				}
				return errReader
			}

			if e := ensureReaderClosed(readerCloser); e != nil {
				return e
			}
		}
	}

	return nil
}

func ExtractPackageSwiftFiles(element *models.UploadElement, fileLocation string, fileExtractor func(name string, r io.ReadCloser) error) error {
	// extract Package Swifts
	if element.MimeType == mimetypes.ApplicationZip {
		r, err := zip.OpenReader(fileLocation)
		if err != nil {
			return err
		}
		defer func() {
			if err == nil {
				err = r.Close()
			}
		}()

		err = ExtractManifestFilesFromZipReader(element, &r.Reader, fileExtractor)
		return err
	}
	return nil
}

func ensureReaderClosed(closer ...io.Closer) error {
	var errors []error
	for _, c := range closer {
		if err := c.Close(); err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		return errors[0]
	}
	return nil
}
