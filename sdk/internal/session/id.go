package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("nulo_%x", b[:])
}

func generateID(used map[string]Entry) string {
	for i := 0; i < 32; i++ {
		var b [4]byte
		_, _ = rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		if _, ok := used[id]; !ok {
			return id
		}
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
