package models

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

func NewUUIDv7() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	millis := uint64(time.Now().UTC().UnixMilli())
	binary.BigEndian.PutUint32(b[0:4], uint32(millis>>16))
	binary.BigEndian.PutUint16(b[4:6], uint16(millis))
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(b[0:4]),
		binary.BigEndian.Uint16(b[4:6]),
		binary.BigEndian.Uint16(b[6:8]),
		binary.BigEndian.Uint16(b[8:10]),
		b[10:16],
	), nil
}

func MustUUIDv7() string {
	id, err := NewUUIDv7()
	if err != nil {
		panic(err)
	}
	return id
}
