package repo

import (
	"OpenSPMRegistry/models"
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
)

func ExtractPackageSwiftFiles(element *models.UploadElement, fileLocation string, packageSwiftReader func(name string, r io.ReadCloser) error) error {
	// extract Package Swifts
	if element.MimeType == "application/zip" {
		r, err := zip.OpenReader(fileLocation)
		if err != nil {
			if r != nil {
				if e := r.Close(); e != nil {
					return e
				}
			}
			return err
		}

		for _, file := range r.File {

			filename := file.FileInfo().Name()
			ext := filepath.Ext(filename)

			if strings.HasPrefix(filename, "Package") && ext == ".swift" {
				readerCloser, err := file.Open()
				if err != nil {
					if e := ensureFileReaderClosed(readerCloser, r); e != nil {
						return e
					}
					return err
				}

				if errReader := packageSwiftReader(filename, readerCloser); errReader != nil {
					if e := ensureFileReaderClosed(readerCloser, r); e != nil {
						return e
					}
					return errReader
				}

				if e := ensureFileReaderClosed(readerCloser, r); e != nil {
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

func ensureFileReaderClosed(readerCloser io.ReadCloser, r *zip.ReadCloser) error {
	if e := readerCloser.Close(); e != nil {
		if e2 := r.Close(); e2 != nil {
			return e2
		}
		return e
	}
	return nil
}
