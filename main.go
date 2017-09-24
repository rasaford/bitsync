package main

import (
	"flag"
	"log"

	"github.com/rasaford/bitsync/defaults"
	"github.com/rasaford/bitsync/torrent"
	"github.com/rasaford/bitsync/torrent/file"
)

var (
	repositoryName = flag.String("name", "default", "Name to use when sharing the selected folder. Other peers need to have the same name set in order have access to the repository")
)

func main() {
	flag.Parse()
	commit()
}

func commit() {
	path, err := file.Create(defaults.DefaultFolder)
	if err != nil {
		log.Fatal(err)
	}
	torrent.Start(path)
}
