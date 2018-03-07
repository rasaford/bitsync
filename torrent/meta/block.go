package meta

import (
	"crypto/sha1"
	"fmt"
	"runtime"

	"github.com/pkg/errors"

	"github.com/rasaford/bitsync/torrent/bitset"
	"github.com/rasaford/bitsync/torrent/file"
)

func CheckPieces(fs file.BlockHandler, totalLength int64, m *MetaInfo) (good, bad int, goodBits *bitset.Bitset, err error) {
	pieceLength := m.Info.PieceLength
	numPieces := int((totalLength + pieceLength - 1) / pieceLength)
	goodBits = bitset.NewBitset(int(numPieces))
	ref := m.Info.Pieces
	if len(ref) != numPieces*sha1.Size {
		err = errors.New("Incorrect Info.Pieces length")
		return
	}
	currentSums, err := hashBlocks(fs, totalLength, m.Info.PieceLength)
	if err != nil {
		return
	}
	for i := 0; i < numPieces; i++ {
		base := i * sha1.Size
		end := base + sha1.Size
		if checkEqual([]byte(ref[base:end]), currentSums[base:end]) {
			good++
			goodBits.Set(int(i))
		} else {
			bad++
		}
	}
	return
}

func checkEqual(ref, current []byte) bool {
	for i := 0; i < len(current); i++ {
		if ref[i] != current[i] {
			return false
		}
	}
	return true
}

type block struct {
	index int64
	hash  string
}

// hashBlocks reads all the individual blocks from the BlockHandler and computes it's
// SHA1 hash.
func hashBlocks(fs file.BlockHandler, totalLength int64, pieceLength int64) []string {
	jobs := make(chan int64)
	results := make(chan block)
	// spawn workers
	for i := 0; i < runtime.NumCPU(); i++ {
		go blockHasher(fs, pieceLength, jobs, results)
	}
	// TODO validate this equation
	numPieces := (totalLength + pieceLength - 1) / pieceLength
	go func() {
		for i := int64(0); i < numPieces; i++ {
			// piece := make([]byte, pieceLength, pieceLength)
			// if i == numPieces-1 {
			// 	piece = piece[0 : totalLength-i*pieceLength]
			// }
			// Ignore errors.
			jobs <- i
		}
		close(jobs)
	}()

	// Merge back the results.
	sums := make([]string, numPieces, numPieces)
	for i := int64(0); i < numPieces; i++ {
		res := <-results
		sums[res.index] = res.hash
	}
	return sums
}

// hashBlock computes the sha1 hash of a block.
func blockHasher(fs file.BlockHandler, pieceSize int64, jobs <-chan int64, result chan<- block) {
	// preallocate the hash & cache structure
	hasher := sha1.New()
	cache := make([]byte, pieceSize, pieceSize)
	for index := range jobs {
		hasher.Reset()
		_, err := fs.ReadBlock(cache, index)
		if err != nil {
			result <- block{index: index, hash: ""}
			continue
		}
		_, err = hasher.Write(cache)
		if err != nil {
			result <- block{index: index, hash: ""}
		} else {
			hash := string(hasher.Sum(nil))
			result <- block{index: index, hash: hash}
		}
	}
}

func CheckPiece(piece []byte, m *MetaInfo, pieceIndex int) (good bool, err error) {
	ref := m.Info.Pieces
	var currentSum []byte
	currentSum, err = computePieceSum(piece)
	if err != nil {
		return
	}
	base := pieceIndex * sha1.Size
	end := base + sha1.Size
	refSha1 := []byte(ref[base:end])
	good = checkEqual(refSha1, currentSum)
	if !good {
		err = fmt.Errorf("reference sha1: %v != piece sha1: %v", refSha1, currentSum)
	}
	return
}

func computePieceSum(piece []byte) (sum []byte, err error) {
	hasher := sha1.New()

	_, err = hasher.Write(piece)
	if err != nil {
		return
	}
	sum = hasher.Sum(nil)
	return
}
