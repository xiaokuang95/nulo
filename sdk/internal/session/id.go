package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// newSessionID returns a time-ordered UUIDv7, as Pi generates session IDs.
func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ms := uint64(time.Now().UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms >> uint((5-i)*8))
	}
	b[6] = 0x70 | (b[6] & 0x0f)
	b[8] = 0x80 | (b[8] & 0x3f)
	encoded := hex.EncodeToString(b[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

// generateID returns a short, collision-checked entry ID, or a UUID when
// every short candidate is taken, as Pi does.
func generateID(used map[string]Entry) string {
	for i := 0; i < 100; i++ {
		var b [4]byte
		_, _ = rand.Read(b[:])
		id := hex.EncodeToString(b[:])
		if _, ok := used[id]; !ok {
			return id
		}
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:])
}
