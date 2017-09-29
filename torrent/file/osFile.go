package file

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
)

//Interface for a provider of filesystems.
type FsProvider interface {
	NewFS(directory string) (FileSystem, error)
}

// a torrent FileSystem that is backed by real OS files
type osFileSystem struct {
	storePath string
}

// A torrent File that is backed by an OS file
type osFile struct {
	filePath string
}

type OsFsProvider struct{}

func (o OsFsProvider) NewFS(directory string) (fs FileSystem, err error) {
	return &osFileSystem{directory}, nil
}

func (o *osFileSystem) Open(file *FileDict) (File, error) {
	// Clean the source path before appending to the storePath. This
	// ensures that source paths that start with ".." can't escape.
	cleanSrcPath := path.Clean("/" + path.Join(file.Path...))[1:]
	fullPath := path.Join(o.storePath, cleanSrcPath)
	// err := ensureDirectory(fullPath)
	// if err != nil {
	// 	return nil, err
	// }
	osfile := &osFile{fullPath}
	if err := osfile.ensureExists(file.Length, file.MD5Sum); err != nil {
		return nil, err
	}
	return osfile, nil
}

func (o *osFileSystem) Close() error {
	return nil
}

func (o *osFile) Close() (err error) {
	return
}

func ensureDirectory(fullPath string) error {
	fullPath = path.Clean(fullPath)
	if !strings.HasPrefix(fullPath, "/") {
		// Transform into absolute path.
		cwd, err := os.Getwd()
		if err != nil {
			log.Fatal("cannot get path to directory", err)
		}
		fullPath = fmt.Sprintf("%s/%s", cwd, fullPath)
	}
	base, _ := path.Split(fullPath)
	if base == "" {
		log.Fatalf("could not find base directory for absolute path %s\n", fullPath)
	}
	return os.MkdirAll(base, 0755)
}

func (o *osFile) ensureExists(length int64, md5Hash string) error {
	name := o.filePath
	st, err := os.Stat(name)
	if err != nil && os.IsNotExist(err) {
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		defer f.Close()
	} else {
		fd, err := os.Open(name)
		if err != nil {
			log.Println(err)
			return nil
		}
		defer fd.Close()
		hash := md5.New()
		_, err = io.Copy(hash, fd)
		if err != nil {
			return err
		}
		readMD5 := string(hash.Sum(nil))
		if st.Size() != length && readMD5 != md5Hash {
			return fmt.Errorf("the file %s exists but is not equal to the indexed one", name)
		}
	}
	err = os.Truncate(name, length)
	if err != nil {
		return fmt.Errorf("could not truncate file")
	}
	return nil
}

func (o *osFile) ReadAt(p []byte, off int64) (n int, err error) {
	file, err := os.OpenFile(o.filePath, os.O_RDWR, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	return file.ReadAt(p, off)
}

func (o *osFile) WriteAt(p []byte, off int64) (n int, err error) {
	file, err := os.OpenFile(o.filePath, os.O_RDWR, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	return file.WriteAt(p, off)
}
