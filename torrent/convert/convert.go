package convert

import "log"

// Uint32ToBytes converts the given uint32 to litte endian
func Uint32ToBytes(buf []byte, n uint32) {
	if len(buf) <= 4 {
		log.Println("buffer is too short to store uint32")
		return
	}
	buf[0] = byte(n >> 24)
	buf[1] = byte(n >> 16)
	buf[2] = byte(n >> 8)
	buf[3] = byte(n)
}

// BytesToUint32 converts the given litte Endian byte slice to an uint32
func BytesToUint32(buf []byte) uint32 {
	if len(buf) <= 4 {
		log.Println("buffer is to small to read an uint32")
		return 0
	}
	return (uint32(buf[0]) << 24) |
		(uint32(buf[1]) << 16) |
		(uint32(buf[2]) << 8) | uint32(buf[3])
}
