package stop

import (
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/agent/stop"
	libagent "github.com/Harrison-Blair/fledge/internal/lib/agent"
	"github.com/Harrison-Blair/fledge/internal/lib/herdr"
)

// The transport limit must outlast the settle wait, or a long --grace is cut
// off locally and reported as an ordinary refusal.
func TestClientTransportCoversGrace(t *testing.T) {
	t.Setenv("HERDR_ENV", "1")
	t.Setenv("HERDR_SOCKET_PATH", "/nonexistent")
	for name, c := range map[string]struct {
		o    stop.Options
		want time.Duration
	}{
		"default": {stop.Options{}, stop.DefaultGrace},
		"60s":     {stop.Options{Grace: time.Minute, GraceSet: true}, time.Minute},
		"0":       {stop.Options{GraceSet: true}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if got := client(c.o).API.(herdr.Client).Timeout; got != c.want+libagent.TransportMargin {
				t.Fatalf("transport timeout %s, want %s", got, c.want+libagent.TransportMargin)
			}
		})
	}
}
