package commit

import (
	"io"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/rasaford/bitsync/torrent/meta"
)

const torrentFileName = "files.bitsync"

// Create creates a torrent file indexing all the files in the given directory.
func Create(dir string) (string, error) {
	path := filepath.Join(dir, torrentFileName)
	path, _ = filepath.Abs(path)
	f, err := createFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	err = writeTorrent(dir, f)
	if err != nil {
		return "", err
	}
	log.Printf("torrent file created at %s\n", path)
	return path, nil
}

func createFile(fileDir string) (*os.File, error) {
	dir, _ := path.Split(fileDir)
	if _, err := os.Stat(dir); err != nil && os.IsNotExist(err) {
		os.Mkdir(dir, 0755)
	}
	f, err := os.Create(fileDir)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func writeTorrent(dir string, w io.Writer) error {
	m, err := meta.Index(nil, dir, ":8000", 0, true)
	if err != nil {
		return err
	}
	// write the data to the writer
	m.Bencode(w)
	if err != nil {
		return err
	}
	return nil
}
