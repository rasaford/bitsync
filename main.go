package main

import (
	"flag"
	"log"

	"github.com/rasaford/bitsync/torrent"
	"github.com/rasaford/bitsync/torrent/commit"
)

var (
	// repositoryName = flag.String("repo", "default", "Name to use when sharing the selected folder. Other peers need to have the same name set in order have access to the repository")
	file = flag.String("file", "", "this is for debugging purposes only")
)

func main() {
	initialize()
	if *file == "" {
		dir, err := commit.Create(".")
		if err != nil {
			log.Fatal(err)
		}
		file = &dir
	}
	torrent.Start(*file)
}

func initialize() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
}
