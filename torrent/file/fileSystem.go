package file

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

// FileSystem is an abstraction for a file system to use for indexing.
type FileSystem interface {
	Open(path []string) (File, error)
	Stat(path []string) (os.FileInfo, error)
	List(path []string) ([]string, error)
}

// OSFileSystem implements the FileSystem interface with methods accessing the
// local fs.
type OSFileSystem struct {
	root string
}

// NewOSFileSystem creates a new FileSystem from the given path.
// The executing user must have permission to read and write to it.
func NewOSFileSystem(directory string) (FileSystem, error) {
	if directory == "" {
		return nil, errors.New("cannot create fileSystem without a nil path")
	}
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create absolute path")
	}
	_, err = os.Stat(abs)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read from path: %s", abs)
	}
	return &OSFileSystem{abs}, nil
}

// Open opens the file and returns the file descriptor
func (o *OSFileSystem) Open(path []string) (File, error) {
	absPath := append([]string{o.root}, path...)
	return os.Open(filepath.Join(absPath...))
}

// Stat returns the file Info for the file at the given Path.
func (o *OSFileSystem) Stat(path []string) (os.FileInfo, error) {
	absPath := append([]string{o.root}, path...)
	return os.Stat(filepath.Join(absPath...))
}

// List returns all the files at the given path.
func (o *OSFileSystem) List(path []string) ([]string, error) {
	d := append([]string{o.root}, path...)
	absPath := filepath.Join(d...)
	fd, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	files, err := fd.Readdirnames(0)
	if err != nil {
		return nil, err
	}
	return files, nil
}
