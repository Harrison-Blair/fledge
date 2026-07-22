package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/flock"
)

const readySignalDir = ".ready"

// ReadySignalPath is the workspace-local fallback used when an agent's
// sandbox cannot connect to the daemon's Unix socket. The file contains only
// the one-use token digest, never the token itself.
func ReadySignalPath(root, flockName, agentName string) string {
	return filepath.Join(flock.Dir(root, flockName), readySignalDir, agentName)
}

// WriteReadySignal atomically publishes a hashed readiness token for the
// daemon to consume. This path exists specifically for sandboxed integrations
// whose command runner can write the workspace but cannot open Unix sockets.
func WriteReadySignal(root, flockName, agentName, token string) error {
	sum := sha256.Sum256([]byte(token))
	digest := hex.EncodeToString(sum[:])
	dir := filepath.Dir(ReadySignalPath(root, flockName, agentName))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ready-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(tmp, digest); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, ReadySignalPath(root, flockName, agentName)); err != nil {
		return err
	}
	ok = true
	return nil
}

// consumeReadySignal validates and removes one published signal. Invalid or
// stale files are consumed too so they cannot spin the readiness loop.
func (d *Daemon) consumeReadySignal(agentName string) (bool, error) {
	path := ReadySignalPath(d.root, d.flockName, agentName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	_, err = d.readyDigest(agentName, strings.TrimSpace(string(data)))
	return true, err
}

// consumeReadySignals resumes signals left while a daemon was restarting.
// The journal-replayed token map is authoritative for which names may become
// ready; unrelated files are never treated as agent declarations.
func (d *Daemon) consumeReadySignals() {
	d.mu.Lock()
	names := make([]string, 0, len(d.readyTokens))
	for name := range d.readyTokens {
		names = append(names, name)
	}
	d.mu.Unlock()
	for _, name := range names {
		if consumed, err := d.consumeReadySignal(name); consumed && err != nil {
			d.debug.Printf("ready signal %s: %v", name, err)
		}
	}
}
