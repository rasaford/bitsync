package main

import (
	"crypto/sha256"
	"flag"
	"os"
	"os/signal"

	"github.com/rasaford/bitsync/defaults"
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
	file.Create(defaults.DefaultFolder)
}

func hashSha256(input string) string {
	hash := sha256.New()
	hash.Write([]byte(input))
	return string(hash.Sum(nil))
}

