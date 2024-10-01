package repo

import "io"

type Repo interface {
	Exists(scope string, packageName string, version string) bool
	Write(scope string, packageName string, version string, reader io.Reader) error
}
