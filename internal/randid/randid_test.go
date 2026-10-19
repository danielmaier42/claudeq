package randid

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

var v4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDIsARandomV4UUID(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		id := UUID()
		if !v4.MatchString(id) {
			t.Fatalf("%q is not a version 4 UUID", id)
		}
		if seen[id] {
			t.Fatalf("%q issued twice", id)
		}
		seen[id] = true
	}
}

func TestHexHasTheRequestedLength(t *testing.T) {
	for _, n := range []int{3, 4, 16} {
		if got := Hex(n); len(got) != 2*n || !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(got) {
			t.Errorf("Hex(%d) = %q", n, got)
		}
	}
}

// Without a random source, ids issued at the same instant must still differ
// and keep their shape.
func TestFallbackStaysUniqueOnAFrozenClock(t *testing.T) {
	read = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	frozen := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return frozen }
	t.Cleanup(func() {
		read = defaultRead
		now = time.Now
	})

	seenUUID, seenHex := map[string]bool{}, map[string]bool{}
	for range 50 {
		u, h := UUID(), Hex(4)
		if !v4.MatchString(u) || len(h) != 8 {
			t.Fatalf("fallback ids lost their shape: %q, %q", u, h)
		}
		if seenUUID[u] || seenHex[h] {
			t.Fatalf("fallback repeated an id: %q, %q", u, h)
		}
		seenUUID[u], seenHex[h] = true, true
	}
}
