// Package filebridge provides a workspace-local fallback for Fledge clients
// whose sandbox forbids Unix-domain sockets. The daemon remains authoritative;
// files are only an RPC transport and carry the same protocol request/response.
package filebridge

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Harrison-Blair/fledge/internal/flock"
	"github.com/Harrison-Blair/fledge/internal/protocol"
)

const dirName = ".rpc"

type pending struct {
	ID      string           `json:"id"`
	Request protocol.Request `json:"request"`
}

// Pending is a claimed request awaiting dispatch by the daemon.
type Pending struct {
	ID      string
	Request protocol.Request
}

func rootDir(root, flockName string) string {
	return filepath.Join(flock.Dir(root, flockName), dirName)
}

func inboxDir(root, flockName string) string {
	return filepath.Join(rootDir(root, flockName), "inbox")
}

func acceptedDir(root, flockName string) string {
	return filepath.Join(rootDir(root, flockName), "accepted")
}

func responseDir(root, flockName string) string {
	return filepath.Join(rootDir(root, flockName), "responses")
}

func alivePath(root, flockName string) string {
	return filepath.Join(rootDir(root, flockName), "alive")
}

// ResetServer discards stale transport files and marks a new daemon available.
func ResetServer(root, flockName string) error {
	dir := rootDir(root, flockName)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, child := range []string{inboxDir(root, flockName), acceptedDir(root, flockName), responseDir(root, flockName)} {
		if err := os.MkdirAll(child, 0o700); err != nil {
			return err
		}
	}
	return os.WriteFile(alivePath(root, flockName), []byte("1\n"), 0o600)
}

// CloseServer removes the availability marker and any completed exchanges.
func CloseServer(root, flockName string) error {
	return os.RemoveAll(rootDir(root, flockName))
}

// Available reports whether a daemon has published the file fallback.
func Available(root, flockName string) bool {
	info, err := os.Stat(alivePath(root, flockName))
	return err == nil && info.Mode().IsRegular()
}

// Submit atomically publishes one request and returns its exchange id.
func Submit(root, flockName string, req protocol.Request) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(pending{ID: id, Request: req})
	if err != nil {
		return "", err
	}
	if err := writeAtomic(inboxDir(root, flockName), id+".json", data); err != nil {
		return "", err
	}
	return id, nil
}

// Await waits for daemon acceptance and then its response. A zero response
// timeout preserves socket transport semantics for long waits and spawns.
func Await(root, flockName, id string, acceptTimeout, responseTimeout time.Duration) (protocol.Response, error) {
	defer Cleanup(root, flockName, id)
	accepted := filepath.Join(acceptedDir(root, flockName), id)
	if err := waitFile(root, flockName, accepted, acceptTimeout); err != nil {
		return protocol.Response{}, err
	}
	response := filepath.Join(responseDir(root, flockName), id+".json")
	if err := waitFile(root, flockName, response, responseTimeout); err != nil {
		return protocol.Response{}, err
	}
	data, err := os.ReadFile(response)
	if err != nil {
		return protocol.Response{}, err
	}
	var resp protocol.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return protocol.Response{}, err
	}
	return resp, nil
}

// Take claims all completely published requests currently in the inbox.
func Take(root, flockName string) ([]Pending, error) {
	entries, err := os.ReadDir(inboxDir(root, flockName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Pending
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(inboxDir(root, flockName), entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var p pending
		if err := json.Unmarshal(data, &p); err != nil || !validID(p.ID) || entry.Name() != p.ID+".json" {
			os.Remove(path)
			continue
		}
		if err := os.WriteFile(filepath.Join(acceptedDir(root, flockName), p.ID), nil, 0o600); err != nil {
			return out, err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return out, err
		}
		out = append(out, Pending{ID: p.ID, Request: p.Request})
	}
	return out, nil
}

// Respond atomically publishes a daemon response.
func Respond(root, flockName, id string, resp protocol.Response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return writeAtomic(responseDir(root, flockName), id+".json", data)
}

// Cleanup removes one exchange's files.
func Cleanup(root, flockName, id string) {
	os.Remove(filepath.Join(inboxDir(root, flockName), id+".json"))
	os.Remove(filepath.Join(acceptedDir(root, flockName), id))
	os.Remove(filepath.Join(responseDir(root, flockName), id+".json"))
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func writeAtomic(dir, name string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
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
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return err
	}
	ok = true
	return nil
}

func waitFile(root, flockName, path string, timeout time.Duration) error {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if !Available(root, flockName) {
			return errors.New("file RPC daemon stopped")
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return fmt.Errorf("file RPC timed out waiting for %s", filepath.Base(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func validID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16
}
