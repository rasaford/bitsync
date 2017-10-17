package commit

import (
	"io"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/rasaford/bitsync/defaults"
	"github.com/rasaford/bitsync/torrent/meta"
)

const torrentFileName = "files" + defaults.Extension

// Create creates a torrent file indexing all the files in the given directory.
func Create(dir string) (string, error) {
	dir, _ = filepath.Abs(dir)
	path := filepath.Join(dir, torrentFileName)
	f, err := createFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	err = createTorrent(dir, f)
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

func createTorrent(dir string, w io.Writer) error {
	m, err := meta.Index(nil, dir, ":8000", 0)
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
