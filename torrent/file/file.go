package file

import (
	"errors"
	"io"
)

// BlockHandler is responsible for reading and writing the blocks from and to the filesystem.
// WritePiece should be called for full, verified pieces only.
//
// All files in the store are opened on creation and only closed, once they have been
// written to entirely.
type BlockHandler interface {
	WriteBlock(block []byte, index int64) (int, error)
	ReadBlock(block []byte, index int64) (int, error)
	io.Closer
}

// File is an interface for local reading and writing to a file.
// All implementations must be goroutine safe.
type File interface {
	io.ReaderAt
	io.WriterAt
	io.Reader
	io.Closer
}

type blockStore struct {
	// filesystem is used for reading and writing the data of the blocks to
	fileSystem FileSystem
	// specifies the byte ranges to write each block to
	offsets []int64
	// files stores handles to all the files that get wirtten to the filesystem.
	// They are stored in increasing global offest order.
	files []fileHandle
	// blockSize is the size of an individual block.
	blockSize int64
}

type fileHandle struct {
	length int64
	file   File
}

const (
	statusUnknown  = 0
	statusHave     = 1
	statusModified = 2
	statusNeed     = 3
)

type FileDict struct {
	// Length of thefile in bytes
	Length int64
	// relative path from the root
	Path []string
	// SHA1hash of the file
	SHA1hash string
}

// NewBlockStore creates a storage interface to be able to write files to disk
func NewBlockStore(info *InfoDict, fileSystem FileSystem) (BlockHandler, int64, error) {
	// ensureFilesExist(info.Files, fileSystem)
	numFiles := len(info.Files)
	if numFiles == 0 {
		// Create dummy Files structure.
		info = &InfoDict{
			Files: make([]FileDict, 0, 8),
		}
		numFiles = 1
	}
	fs := &blockStore{
		fileSystem: fileSystem,
		offsets:    make([]int64, numFiles),
		files:      make([]fileHandle, numFiles),
		blockSize:  info.PieceLength,
	}
	// Compare between what's in the InfoDict and on the fileSystem
	totalSize := int64(0)
	for i, src := range info.Files {
		file, err := fileSystem.Open(src.Path)
		if err != nil {
			// Close all files opened up to now.
			for j := 0; j < i; j++ {
				fs.files[j].file.Close()
			}
			return nil, 0, err
		}
		fs.files[i].file = file
		fs.files[i].length = src.Length
		fs.offsets[i] = totalSize
		totalSize += src.Length
	}
	return fs, totalSize, nil
}

func ensureFilesExist(files []FileDict, fs FileSystem) {

}

func (f *blockStore) find(offset int64) int {
	// Binary search
	offsets := f.offsets
	low := 0
	high := len(offsets)
	for low < high-1 {
		probe := (low + high) / 2
		entry := offsets[probe]
		if offset < entry {
			high = probe
		} else {
			low = probe
		}
	}
	return low
}

// func (f *blockStore) ReadAt(p []byte, off int64) (n int, err error) {
// 	index := f.find(off)
// 	for len(p) > 0 && index < len(f.offsets) {
// 		chunk := int64(len(p))
// 		entry := &f.files[index]
// 		itemOffset := off - f.offsets[index]
// 		if itemOffset < entry.length {
// 			space := entry.length - itemOffset
// 			if space < chunk {
// 				chunk = space
// 			}
// 			var nThisTime int
// 			nThisTime, err = entry.file.ReadAt(p[0:chunk], itemOffset)
// 			n = n + nThisTime
// 			if err != nil {
// 				return
// 			}
// 			p = p[nThisTime:]
// 			off += int64(nThisTime)
// 		}
// 		index++
// 	}
// 	// At this point if there's anything left to read it means we've run off the
// 	// end of the file store. Read zeros. This is defined by the bittorrent protocol.
// 	for i := range p {
// 		p[i] = 0
// 	}
// 	return
// }

func (s *blockStore) WriteBlock(cache []byte, piece int64) (n int, err error) {
	offset := int64(piece) * s.pieceSize
	index := s.find(offset)
	for len(cache) > 0 && index < len(s.offsets) {
		chunk := int64(len(cache))
		entry := &s.files[index]
		itemOffset := offset - s.offsets[index]
		if itemOffset < entry.length {
			space := entry.length - itemOffset
			if space < chunk {
				chunk = space
			}
			var nThisTime int
			nThisTime, err = entry.file.WriteAt(cache[0:chunk], itemOffset)
			n += nThisTime
			if err != nil {
				return
			}
			cache = cache[nThisTime:]
			offset += int64(nThisTime)
		}
		index++
	}
	// At this point if there's anything left to write it means we've run off the
	// end of the file store. Check that the data is zeros.
	// This is defined by the bittorrent protocol.
	for i := range cache {
		if cache[i] != 0 {
			err = errors.New("unexpected non-zero data at end of store")
			n += i
			return
		}
	}
	n += len(cache)
	return
}

func (f *blockStore) ReadBlock(cache []byte, index int64) (int, error) {
	return 0, nil
}

func (f *blockStore) Close() (err error) {
	for i := range f.files {
		f.files[i].file.Close()
	}
	// if f.fileSystem != nil {
	// 	err = f.fileSystem.Close()
	// }
	return
}
