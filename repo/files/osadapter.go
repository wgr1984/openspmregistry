package files

import (
	"io/fs"
	"os"
	"path/filepath"
)

// OsAdapter is an interface that wraps the basic operations
// that a repository uses from the os package (file system operations)
type OsAdapter interface {
	// Stat returns a [FileInfo] describing the named file.
	// If there is an error, it will be of type [*PathError].
	Stat(name string) (fs.FileInfo, error)

	// MkdirAll creates a directory named path,
	// along with any necessary parents, and returns nil,
	// or else returns an error.
	// The permission bits perm (before umask) are used for all
	// directories that MkdirAll creates.
	// If path is already a directory, MkdirAll does nothing
	// and returns nil.
	MkdirAll(path string, perm fs.FileMode) error

	// Open opens the named file for reading. If successful, methods on
	// the returned file can be used for reading; the associated file
	// descriptor has mode O_RDONLY.
	// If there is an error, it will be of type *PathError.
	Open(name string) (*os.File, error)

	// Create creates or truncates the named file. If the file already exists,
	// it is truncated. If the file does not exist, it is created with mode 0o666
	// (before umask). If successful, methods on the returned File can
	// be used for I/O; the associated file descriptor has mode O_RDWR.
	// If there is an error, it will be of type *PathError.
	Create(name string) (*os.File, error)

	// WalkDir walks the file tree rooted at root, calling fn for each file or
	// directory in the tree, including root.
	//
	// All errors that arise visiting files and directories are filtered by fn:
	// see the [fs.WalkDirFunc] documentation for details.
	//
	// The files are walked in lexical order, which makes the output deterministic
	// but requires WalkDir to read an entire directory into memory before proceeding
	// to walk that directory.
	//
	// WalkDir does not follow symbolic links.
	//
	// WalkDir calls fn with paths that use the separator character appropriate
	// for the operating system. This is unlike [io/fs.WalkDir], which always
	// uses slash separated paths.
	WalkDir(root string, fn fs.WalkDirFunc) error
}

// osAdapterDefault is the default implementation of the OsAdapter interface
// that uses the os package
type osAdapterDefault struct{}

func (*osAdapterDefault) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (*osAdapterDefault) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (*osAdapterDefault) Open(name string) (*os.File, error) {
	return os.Open(name)
}

func (*osAdapterDefault) Create(name string) (*os.File, error) {
	return os.Create(name)
}

func (*osAdapterDefault) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}
