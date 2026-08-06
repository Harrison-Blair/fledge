package watchproc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Harrison-Blair/fledge/internal/statedir"
	"github.com/Harrison-Blair/fledge/internal/trace"
)

const storedRecord = `{"at":"2026-08-05T20:41:09Z","kind":"message","origin":"coord","target":"worker","ref":"m-9f2c","body":"rerun the build"}`

func TestLineRendererDecodesForPeopleAndPassesJSONThrough(t *testing.T) {
	t.Parallel()

	human := lineRenderer(false, false)([]byte(storedRecord))
	if !strings.Contains(string(human), "message") || !strings.Contains(string(human), "coord -> worker") {
		t.Fatalf("human render = %q", human)
	}
	if strings.Contains(string(human), `"kind"`) {
		t.Fatalf("human render leaked JSON: %q", human)
	}
	if got := lineRenderer(true, false)([]byte(storedRecord)); string(got) != storedRecord {
		t.Fatalf("json render = %q, want the stored line verbatim", got)
	}
	// A dispatcher upgraded mid-session left plain text behind it.
	if got := lineRenderer(false, false)([]byte("dispatcher ready")); string(got) != "dispatcher ready" {
		t.Fatalf("legacy line = %q, want pass-through", got)
	}
	if colored := lineRenderer(false, true)([]byte(storedRecord)); !bytes.Contains(colored, []byte("\x1b[")) {
		t.Fatalf("colored render = %q", colored)
	}
}

func TestLineRendererJSONDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	line := []byte(storedRecord)
	rendered := lineRenderer(true, false)(line)
	if &rendered[0] == &line[0] {
		t.Fatal("JSON rendering aliases the stored log line")
	}
}

// The log is the follower's only source, so it always holds JSON even while the
// attached terminal is reading the human form.
func TestRecordSinkStoresJSONAndRendersToTheTerminal(t *testing.T) {
	t.Parallel()

	record := trace.Record{At: time.Date(2026, 8, 5, 20, 41, 9, 0, time.UTC), Kind: "message",
		Origin: "coord", Target: "worker", Ref: "m-9f2c", Body: "rerun the build"}
	var log, output bytes.Buffer
	emit, flush := recordSink(&log, &output, false, false, false)
	emit(record)
	flush()
	if !strings.HasPrefix(log.String(), `{"at":`) || !strings.HasSuffix(log.String(), "\n") {
		t.Fatalf("stored line = %q", log.String())
	}
	if strings.Contains(output.String(), `"kind"`) || !strings.Contains(output.String(), "coord -> worker") {
		t.Fatalf("terminal line = %q", output.String())
	}

	log.Reset()
	output.Reset()
	emit, flush = recordSink(&log, &output, false, true, false)
	emit(record)
	flush()
	if log.String() != output.String() {
		t.Fatalf("--json terminal = %q, stored = %q", output.String(), log.String())
	}
	if bytes.Count(log.Bytes(), []byte{'\n'}) != 1 || !json.Valid(bytes.TrimSuffix(log.Bytes(), []byte{'\n'})) {
		t.Fatalf("--json stored line is not one complete JSON record: %q", log.String())
	}

	log.Reset()
	output.Reset()
	emit, flush = recordSink(&log, &output, true, false, false)
	emit(record)
	flush()
	if output.Len() != 0 || log.Len() == 0 {
		t.Fatalf("daemon wrote %q to its terminal", output.String())
	}
}

func TestRecordSinkTerminalDoesNotBlockEmit(t *testing.T) {
	t.Parallel()

	const records = 128
	var log bytes.Buffer
	output := newBlockingWriter()
	emit, flush := recordSink(&log, output, false, true, false)
	done := make(chan struct{})
	go func() {
		for i := range records {
			emit(trace.Record{At: time.Date(2026, 8, 5, 20, 41, 9, 0, time.UTC), Kind: "message",
				Origin: "coord", Target: "worker", Ref: fmt.Sprintf("m-%d", i), Body: "rerun the build"})
		}
		close(done)
	}()
	select {
	case <-output.entered:
	case <-time.After(time.Second):
		close(output.release)
		t.Fatal("terminal writer was never reached")
	}
	blocked := false
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		blocked = true
	}
	close(output.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit did not finish after releasing the terminal writer")
	}
	flushed := make(chan struct{})
	go func() {
		flush()
		close(flushed)
	}()
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("terminal writer goroutine did not stop after flushing")
	}
	if blocked {
		t.Error("emit blocked on the terminal writer")
	}
	stored := strings.Split(strings.TrimSuffix(log.String(), "\n"), "\n")
	if len(stored) != records {
		t.Errorf("stored %d lines, want %d", len(stored), records)
	} else {
		for i, line := range stored {
			record, ok := trace.Decode([]byte(line))
			if !ok || record.Ref != fmt.Sprintf("m-%d", i) {
				t.Fatalf("stored line %d = %q", i, line)
			}
		}
	}
	visible, dropNotes, dropCount := 0, 0, 0
	for _, line := range strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n") {
		var count int
		if _, err := fmt.Sscanf(line, "dropped %d lines", &count); err == nil {
			dropNotes++
			dropCount += count
			continue
		}
		if _, ok := trace.Decode([]byte(line)); ok {
			visible++
		}
	}
	if dropNotes != 1 || dropCount == 0 {
		t.Errorf("terminal reported %d drop notes covering %d lines, want one non-empty note", dropNotes, dropCount)
	}
	if visible+dropCount != records {
		t.Errorf("terminal accounted for %d visible + %d dropped lines, want %d", visible, dropCount, records)
	}
}

// A follower must show the same trace the attached dispatcher shows, which is
// the whole reason the log stores records instead of rendered text.
func TestFollowLogRendersStoredRecords(t *testing.T) {
	t.Parallel()

	for name, jsonMode := range map[string]bool{"human": false, "json": true} {
		t.Run(name, func(t *testing.T) {
			root := sessionRoot(t)
			logPath := filepath.Join(statedir.Session(root, testSession), LogFilename)
			if err := os.WriteFile(logPath, []byte(storedRecord+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			output := &syncBuffer{}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- followLog(ctx, root, testSession, output, lineRenderer(jsonMode, false)) }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("followLog did not exit")
			}
			if jsonMode {
				if strings.TrimSpace(output.String()) != storedRecord {
					t.Fatalf("--json output = %q", output.String())
				}
				return
			}
			if strings.Contains(output.String(), `"kind"`) || !strings.Contains(output.String(), "coord -> worker") {
				t.Fatalf("output = %q", output.String())
			}
			if !strings.HasSuffix(output.String(), "\n") {
				t.Fatalf("output is not newline terminated: %q", output.String())
			}
		})
	}
}

type blockingWriter struct {
	entered  chan struct{}
	release  chan struct{}
	enter    sync.Once
	mu       sync.Mutex
	contents bytes.Buffer
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.enter.Do(func() { close(w.entered) })
	<-w.release
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.contents.Write(p)
}

func (w *blockingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.contents.String()
}
