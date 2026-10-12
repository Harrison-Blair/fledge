package identitytest

import (
	"context"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/lib/identity"
)

// Each identity.Existing call counts once; other Git commands do not count.
func TestCountRootsCountsEachExisting(t *testing.T) {
	cwd := Repository(t)
	roots := CountRoots(t)
	if got := roots(); got != 0 {
		t.Fatalf("counted %d before any lookup", got)
	}
	for want := 1; want <= 2; want++ {
		if _, err := identity.Existing(context.Background(), cwd); err != nil {
			t.Fatal(err)
		}
		if got := roots(); got != want {
			t.Fatalf("counted %d, want %d", got, want)
		}
	}
}
