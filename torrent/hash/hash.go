package hash

import (
	"crypto/sha1"
	"io"
	"log"
)

// SHA1 returns the sha1 hash of the bytes provided by the reader.
func SHA1(file io.Reader) string {
	hash := sha1.New()
	var b []byte
	if _, err := file.Read(b); err != nil {
		log.Fatal("cannot read from the given io.Reader")
	}
	hash.Write(b)
	return string(hash.Sum(nil))
}
