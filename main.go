package main

import (
	"flag"
	"log"
	"os"

	"github.com/rasaford/bitsync/torrent"
	"github.com/rasaford/bitsync/torrent/commit"
)

func main() {
	if len(os.Args) < 2 {
		log.Panic("need to specify a directory for syncing")
	}
	repoDir := os.Args[1]
	initialize()
	dir, err := commit.Create(repoDir)
	if err != nil {
		log.Fatal(err)
	}
	torrent.Start(dir)
}

func initialize() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()
}
