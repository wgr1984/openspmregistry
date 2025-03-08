package files

import (
	"OpenSPMRegistry/mimetypes"
	"OpenSPMRegistry/models"
	"archive/zip"
	"io"
	"path/filepath"
	"strings"
)

func ExtractPackageSwiftFiles(element *models.UploadElement, fileLocation string, packageSwiftReader func(name string, r io.ReadCloser) error) error {
	// extract Package Swifts
	if element.MimeType == mimetypes.ApplicationZip {
		r, err := zip.OpenReader(fileLocation)
		if err != nil {
			return err
		}

		for _, file := range r.File {

			filename := file.FileInfo().Name()
			ext := filepath.Ext(filename)

			if strings.HasPrefix(filename, "Package") && ext == ".swift" {
				readerCloser, err := file.Open()
				if err != nil {
					if e := ensureReaderClosed(r); e != nil {
						return e
					}
					return err
				}

				if errReader := packageSwiftReader(filename, readerCloser); errReader != nil {
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
