package watchproc

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/Harrison-Blair/fledge/internal/trace"
)

// lineRenderer turns one stored log line into what a reader asked to see. The
// log holds JSON, so --json is a pass-through and the default decodes. A line
// that is not a record — output written by an older Fledge into the same log —
// is passed through unchanged rather than dropped.
func lineRenderer(jsonMode, color bool) func([]byte) []byte {
	if jsonMode {
		return func(line []byte) []byte { return append([]byte(nil), line...) }
	}
	return func(line []byte) []byte {
		record, ok := trace.Decode(line)
		if !ok {
			return append([]byte(nil), line...)
		}
		return []byte(trace.Human(record, color))
	}
}

const terminalBufferSize = 64

// recordSink writes each record twice: JSON into the dispatcher log, so a
// second `fledge watch` can follow it, and the reader's chosen rendering to the
// attached terminal. A daemon has no terminal, so it only writes the log.
func recordSink(logFile, output io.Writer, daemon, jsonMode, color bool) (func(trace.Record), func()) {
	store := func(record trace.Record) []byte {
		line, err := trace.JSON(record)
		if err != nil {
			return nil
		}
		_, _ = logFile.Write(append(line, '\n'))
		return line
	}
	if daemon {
		return func(record trace.Record) { store(record) }, func() {}
	}

	render := lineRenderer(jsonMode, color)
	terminal := make(chan []byte, terminalBufferSize)
	var dropped atomic.Uint64
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for line := range terminal {
			if _, err := output.Write(line); err != nil {
				continue
			}
			if count := dropped.Swap(0); count != 0 {
				_, _ = fmt.Fprintf(output, "dropped %d lines\n", count)
			}
		}
	}()
	emit := func(record trace.Record) {
		line := store(record)
		if line == nil {
			return
		}
		rendered := append(render(line), '\n')
		select {
		case terminal <- rendered:
		default:
			dropped.Add(1)
		}
	}
	flush := func() {
		close(terminal)
		writer.Wait()
	}
	return emit, flush
}
