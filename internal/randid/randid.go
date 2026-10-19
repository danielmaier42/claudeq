// Package randid issues the random identifiers dispatchq names things with:
// session ids handed to a harness before it has seen them, and the short
// suffixes that keep run, task and inbox ids apart.
package randid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// read is the random source; tests replace it to reach the fallback.
var read = defaultRead

func defaultRead(b []byte) (int, error) { return rand.Read(b) }

// now is the clock the fallback mixes in; tests replace it with a fixed one.
var now = time.Now

// counter makes every fallback value distinct within one process, even when
// the clock does not move between two calls.
var counter atomic.Uint64

// fill puts n random bytes into b. Should the system's random source fail, it
// derives them from the clock and a process-wide counter instead of a
// constant, so two ids issued at once still differ.
func fill(b []byte) {
	if _, err := read(b); err == nil {
		return
	}
	var seed [16]byte
	binary.BigEndian.PutUint64(seed[:8], uint64(now().UnixNano()))
	binary.BigEndian.PutUint64(seed[8:], counter.Add(1))
	// Hashed, so every byte depends on both halves, whatever n is.
	for i := 0; i < len(b); i += sha256.Size {
		sum := sha256.Sum256(append(seed[:], byte(i/sha256.Size)))
		copy(b[i:], sum[:])
	}
}

// UUID returns a random version 4 UUID.
func UUID() string {
	var b [16]byte
	fill(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Hex returns n random bytes as 2n lowercase hex digits.
func Hex(n int) string {
	b := make([]byte, n)
	fill(b)
	return hex.EncodeToString(b)
}
