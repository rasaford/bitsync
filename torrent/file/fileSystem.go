package file

import (
	"io"
	"log"
	"os"
	"path"

	"github.com/jackpal/Taipei-Torrent/torrent"
	"github.com/rasaford/bitsync/defaults"
)

// Create creates a torrent file indexing all the files in the given directory.
func Create(dir string) (string, error) {
	path := path.Join(dir, defaults.TorrentFileName)
	f, err := createFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	writeTorrent(dir, f)
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

func writeTorrent(dir string, w io.Writer) {
	m, err := torrent.CreateMetaInfoFromFileSystem(nil, dir, ":8000", 0, true)
	if err != nil {
		log.Fatal("cannot create metadata", err)
	}
	m.Bencode(w)
	if err != nil {
		log.Fatal("cannot encode the metadata", err)
	}
}
