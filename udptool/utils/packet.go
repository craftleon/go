package utils

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math/rand"
	"strconv"
	"time"
)

const (
	MagicWord         = "udpZ"
	ChecksumHeaderLen = 4
	IndexHeaderLen    = 8
	SizeHeaderLen     = 2
	HeaderLen         = ChecksumHeaderLen + IndexHeaderLen + SizeHeaderLen
	MaxPacketSize     = 4096
)

type PacketHeader struct {
	Checksum uint32
	Index    uint64
	Size     uint16
}

type Message struct {
	Header *PacketHeader
	Packet []byte
}

func CheckPacket(packet []byte) (*PacketHeader, error) {
	var header PacketHeader
	br := bytes.NewReader(packet[:HeaderLen])
	err := binary.Read(br, binary.LittleEndian, &header)
	if err != nil {
		return nil, errors.New("packet header read errir.")
	}

	if int(header.Size)+HeaderLen != len(packet) {
		return nil, errors.New("packet size incorrect.")
	}

	indexStr := strconv.FormatUint(header.Index, 10)
	hash := crc32.NewIEEE()
	hash.Write([]byte(MagicWord))
	hash.Write([]byte(indexStr))
	s := hash.Sum32()

	if s != header.Checksum {
		return nil, errors.New("checksum incorrect.")
	}

	return &header, nil
}

func MakePacket(index uint64, size uint16) []byte {
	indexStr := strconv.FormatUint(index, 10)
	hash := crc32.NewIEEE()
	hash.Write([]byte(MagicWord))
	hash.Write([]byte(indexStr))
	s := hash.Sum32()

	header := &PacketHeader{
		Checksum: s,
		Index:    index,
		Size:     size,
	}

	packet := make([]byte, HeaderLen+size)
	bw := bytes.NewBuffer(packet[:0])
	err := binary.Write(bw, binary.LittleEndian, header)
	if err != nil {
		return nil
	}
	if size > 0 {
		rand.Seed(time.Now().UnixNano())
		rand.Read(packet[HeaderLen:])
	}

	return packet
}
