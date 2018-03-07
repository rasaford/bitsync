package file

import (
	"os"
)

//Interface for a provider of filesystems.
type FsProvider interface {
	NewFS(directory string) (FileSystem, error)
}

type OsFsProvider struct{}

// Implementation of the File interface backed by an operating system file
type osFile struct {
	*os.File
}

// func ensureDirectory(fullPath string) error {
// 	fullPath = path.Clean(fullPath)
// 	if !strings.HasPrefix(fullPath, "/") {
// 		// Transform into absolute path.
// 		cwd, err := os.Getwd()
// 		if err != nil {
// 			log.Fatal("cannot get path to directory", err)
// 		}
// 		fullPath = fmt.Sprintf("%s/%s", cwd, fullPath)
// 	}
// 	base, _ := path.Split(fullPath)
// 	if base == "" {
// 		log.Fatalf("could not find base directory for absolute path %s\n", fullPath)
// 	}
// 	return os.MkdirAll(base, 0755)
// }

// func (o *osFile) ensureExists(length int64, md5Hash string) error {
// 	name := o.filePath
// 	st, err := os.Stat(name)
// 	if err != nil && os.IsNotExist(err) {
// 		f, err := os.Create(name)
// 		if err != nil {
// 			return err
// 		}
// 		defer f.Close()
// 	} else {
// 		log.Println(name)
// 		fd, err := os.Open(name)
// 		if err != nil {
// 			log.Println(err)
// 			return nil
// 		}
// 		defer fd.Close()
// 		readMD5 := hash.SHA1(fd)
// 		if st.Size() != length && readMD5 != md5Hash {
// 			return fmt.Errorf("the file %s exists but is not equal to the indexed one", name)
// 		}
// 	}
// 	err = os.Truncate(name, length)
// 	if err != nil {
// 		return fmt.Errorf("could not truncate file")
// 	}
// 	return nil
// }

// func (o *osFile) ReadAt(p []byte, off int64) (n int, err error) {
// 	file, err := os.OpenFile(o.filePath, os.O_RDWR, 0600)
// 	if err != nil {
// 		return
// 	}
// 	defer file.Close()
// 	return file.ReadAt(p, off)
// }

// func (o *osFile) WriteAt(p []byte, off int64) (n int, err error) {
// 	file, err := os.OpenFile(o.filePath, os.O_RDWR, 0600)
// 	if err != nil {
// 		return
// 	}
// 	defer file.Close()
// 	return file.WriteAt(p, off)
// }
