package bitset

import (
	"log"
)

// As defined by the bittorrent protocol, this bitset is big-endian, such that
// the high bit of the first byte is block 0
type Bitset struct {
	bytes    []byte
	length   int
	endIndex int
	endMask  byte // Which bits of the last byte are valid
}

func NewBitset(n int) *Bitset {
	endIndex, endOffset := n>>3, n&7
	endMask := ^byte(255 >> byte(endOffset))
	if endOffset == 0 {
		endIndex = -1
	}
	return &Bitset{make([]byte, (n+7)>>3), n, endIndex, endMask}
}

// Creates a new bitset from a given byte stream. Returns nil if the
// data is invalid in some way.
func NewBitsetFromBytes(n int, data []byte) *Bitset {
	bitset := NewBitset(n)
	if len(bitset.bytes) != len(data) {
		return nil
	}
	copy(bitset.bytes, data)
	if bitset.endIndex >= 0 && bitset.bytes[bitset.endIndex]&(^bitset.endMask) != 0 {
		return nil
	}
	return bitset
}

func (b *Bitset) Set(index int) {
	b.checkRange(index)
	b.bytes[index>>3] |= byte(128 >> byte(index&7))
}

func (b *Bitset) Clear(index int) {
	b.checkRange(index)
	b.bytes[index>>3] &= ^byte(128 >> byte(index&7))
}

func (b *Bitset) IsSet(index int) bool {
	b.checkRange(index)
	return (b.bytes[index>>3] & byte(128>>byte(index&7))) != 0
}

func (b *Bitset) Len() int {
	return b.length
}

func (b *Bitset) InRange(index int) bool {
	return 0 <= index && index < b.length
}

func (b *Bitset) checkRange(index int) {
	if !b.InRange(index) {
		log.Fatalf("Index %d out of range 0..%d.", index, b.length)
	}
}

func (b *Bitset) AndNot(b2 *Bitset) {
	if b.length != b2.length {
		log.Fatalf("Unequal bitset sizes %d != %d", b.length, b2.length)
	}
	for i := 0; i < len(b.bytes); i++ {
		b.bytes[i] = b.bytes[i] & ^b2.bytes[i]
	}
	b.clearEnd()
}

func (b *Bitset) clearEnd() {
	if b.endIndex >= 0 {
		b.bytes[b.endIndex] &= b.endMask
	}
}

func (b *Bitset) IsEndValid() bool {
	if b.endIndex >= 0 {
		return (b.bytes[b.endIndex] & b.endMask) == 0
	}
	return true
}

// TODO: Make this fast
func (b *Bitset) FindNextSet(index int) int {
	for i := index; i < b.length; i++ {
		if (b.bytes[i>>3] & byte(128>>byte(i&7))) != 0 {
			return i
		}
	}
	return -1
}

// TODO: Make this fast
func (b *Bitset) FindNextClear(index int) int {
	for i := index; i < b.length; i++ {
		if (b.bytes[i>>3] & byte(128>>byte(i&7))) == 0 {
			return i
		}
	}
	return -1
}

func (b *Bitset) Bytes() []byte {
	return b.bytes
}
