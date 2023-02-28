package utils

import (
	"hash/crc32"
)

const (
	MagicWord = "udpZ"
	HeaderLen = 4
)

func StringChecksum(content string) string {
	hash := crc32.NewIEEE()
	hash.Write([]byte(MagicWord))
	hash.Write([]byte(content))
	s := hash.Sum32()
	buf := []byte{
		byte(s >> 24), byte(s >> 16), byte(s >> 8), byte(s),
	}

	return string(buf[:])
}
