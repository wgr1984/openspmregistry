package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
	"io"
	"path"
	"strings"
)

func ExtractPackageSwiftFiles(element *models.UploadElement, fileLocation string, fileExtractor func(name string, r io.ReadCloser) error) error {
	// extract Package Swifts
	if element.MimeType == mimetypes.ApplicationZip {
		r, err := zip.OpenReader(fileLocation)
		if err != nil {
			return err
		}

		for _, file := range r.File {
			if file.FileInfo().IsDir() {
				continue
			}

			cleanName := path.Clean(file.Name)
			dir := path.Dir(cleanName)
			base := path.Base(cleanName)
			ext := path.Ext(base)

			// Only consider manifests at the archive root or within a single top-level directory
			// whose name starts with the scope (e.g., "ext.RxSwift/Package.swift").
			// if dir != "." && dir != "/" {
			if !strings.HasPrefix(dir, element.Scope) {
				continue
			}
			// Disallow further nesting (e.g., "ext.RxSwift/Tests/Package.swift")
			if strings.Contains(strings.TrimPrefix(dir, element.Scope), "/") {
				continue
			}

			// Extract Package.swift files
			if strings.HasPrefix(base, "Package") && ext == ".swift" {
				readerCloser, err := file.Open()
				if err != nil {
					if e := ensureReaderClosed(r); e != nil {
						return e
					}
					return err
				}

				if errReader := fileExtractor(base, readerCloser); errReader != nil {
					if e := ensureReaderClosed(readerCloser, r); e != nil {
						return e
					}
					return errReader
				}

				if e := ensureReaderClosed(readerCloser); e != nil {
					return e
				}
			}

			// Extract Package.json file
			if base == "Package.json" {
				readerCloser, err := file.Open()
				if err != nil {
					if e := ensureReaderClosed(r); e != nil {
						return e
					}
					return err
				}

				if errReader := fileExtractor(base, readerCloser); errReader != nil {
					if e := ensureReaderClosed(readerCloser, r); e != nil {
						return e
					}
					return errReader
				}

				if e := ensureReaderClosed(readerCloser); e != nil {
					return e
				}
			}
		}

		if e := r.Close(); e != nil {
			return e
		}
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
