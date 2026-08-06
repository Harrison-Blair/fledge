package trace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Harrison-Blair/fledge/internal/messaging"
)

// Read returns every complete ledger line appended since offset, along with the
// offset to resume from. A torn trailing line is left for the next call and an
// undecodable one is skipped: a diagnostic tail must never be able to end the
// dispatcher that feeds it.
func Read(path string, offset int64) ([]messaging.LedgerEntry, int64, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, offset, nil
	}
	if err != nil {
		return nil, offset, fmt.Errorf("open session ledger %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, offset, fmt.Errorf("inspect session ledger %q: %w", path, err)
	}
	// A ledger shorter than the offset was replaced, so resume from its head
	// rather than from a position that now points into another session's line.
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("seek session ledger %q: %w", path, err)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, fmt.Errorf("read session ledger %q: %w", path, err)
	}
	end := bytes.LastIndexByte(contents, '\n') + 1
	if end == 0 {
		return nil, offset, nil
	}
	var entries []messaging.LedgerEntry
	for line := range bytes.SplitSeq(contents[:end-1], []byte{'\n'}) {
		if entry, ok := messaging.DecodeLedgerLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries, offset + int64(end), nil
}
