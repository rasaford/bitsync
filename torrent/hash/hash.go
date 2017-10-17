package hash

import (
	"crypto/md5"
	"encoding/base64"
	"io"
)

// FileHash returns a base64 encoded version md5 hash of the file at the given reader.
func FileHash(file io.Reader) string {
	hash := md5.New()
	_, err := io.Copy(hash, file)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}
