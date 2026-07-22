// Package herdr resolves and launches Herdr sessions by shelling out to the
// herdr CLI. Only the session-lifecycle surface fledge needs is covered:
// listing sessions, starting a headless one, and probing whether one is up.
//
// Verified live against herdr 0.7.4 / protocol 16. Protocol 16 emits no
// session-lifecycle event — the event stream is pane, tab, workspace and
// worktree scoped only — so a session ending is observable solely by its API
// socket going away. Probing is therefore the only mechanism, not a fallback.
package herdr

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// SessionEnv names the Herdr session a fledge daemon is bound to. fledge start
// sets it on the daemon it launches; an unset value leaves the daemon unbound.
const SessionEnv = "FLEDGE_HERDR_SESSION"

// Session is one entry of `herdr session list --json`.
type Session struct {
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	Default    bool   `json:"default"`
	SocketPath string `json:"socket_path"`
}

// List returns every session herdr knows about, running or not.
func List() ([]Session, error) {
	out, err := exec.Command("herdr", "session", "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("herdr session list: %w", err)
	}
	var payload struct {
		Sessions []Session `json:"sessions"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("herdr session list: %w", err)
	}
	return payload.Sessions, nil
}

// Find returns the named session. A name of "" means the default session.
func Find(name string) (Session, bool, error) {
	sessions, err := List()
	if err != nil {
		return Session{}, false, err
	}
	for _, s := range sessions {
		if (name == "" && s.Default) || s.Name == name {
			return s, true, nil
		}
	}
	return Session{}, false, nil
}

// Up reports whether a session is listening on socket. Herdr removes the
// socket file when a session ends, so a failed dial is a definitive answer.
func Up(socket string) bool {
	if socket == "" {
		return false
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Ensure returns the named session, starting a headless server for it if it is
// not already running. A name of "" targets the default session.
//
// env is added to the environment of a session this call starts. Panes inherit
// the session server's environment (verified on 0.7.4), so this is how
// per-session variables reach every agent without tagging each spawn. It
// cannot apply to an already-running session, which is what started reports:
// false means the caller attached to a session whose environment is whatever
// it was launched with.
// dir is where a session this call starts is rooted: the server's working
// directory, which every pane it opens inherits.
func Ensure(name string, env []string, dir string) (s Session, started bool, err error) {
	s, found, err := Find(name)
	if err != nil {
		return Session{}, false, err
	}
	if found && Up(s.SocketPath) {
		return s, false, nil
	}
	if name == "" {
		return Session{}, false, fmt.Errorf("no default herdr session running; start one with `herdr`")
	}
	s, err = start(name, env, dir)
	return s, err == nil, err
}

// Recreate replaces any session record named name with a fresh headless
// server. Callers use this only for sessions they own: an old managed session
// can outlive its daemon and retain pane identities that the new daemon cannot
// adopt safely. Stopping and deleting it before launch restores a clean
// workspace and the requested environment.
func Recreate(name string, env []string, dir string) (Session, error) {
	if name == "" {
		return Session{}, fmt.Errorf("cannot recreate the default herdr session")
	}
	if err := Remove(name); err != nil {
		return Session{}, err
	}
	return start(name, env, dir)
}

// Remove stops and deletes a named session when its record exists. It is
// idempotent so cleanup paths can remove an associated managed session without
// first having to distinguish a live server, a stopped record, and no record.
func Remove(name string) error {
	if name == "" {
		return fmt.Errorf("cannot remove the default herdr session")
	}
	s, found, err := Find(name)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if Up(s.SocketPath) {
		if err := Stop(name); err != nil {
			return err
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			stopped, stillFound, findErr := Find(name)
			if findErr == nil && (!stillFound || !stopped.Running || !Up(stopped.SocketPath)) {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("herdr session %s did not stop", name)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return Delete(name)
}

// Stop ends the named session. Herdr tears down the session's panes and
// removes its API socket; anything watching that socket sees it go away.
func Stop(name string) error {
	if out, err := exec.Command("herdr", "session", "stop", name).CombinedOutput(); err != nil {
		return fmt.Errorf("herdr session stop %s: %w: %s", name, err, out)
	}
	return nil
}

// Delete removes a stopped session's record and directory from herdr's
// session list. Herdr refuses to delete a running session, so callers stop
// the session first.
func Delete(name string) error {
	if out, err := exec.Command("herdr", "session", "delete", name).CombinedOutput(); err != nil {
		return fmt.Errorf("herdr session delete %s: %w: %s", name, err, out)
	}
	return nil
}

// start launches `herdr --session <name> server` detached in dir and waits
// for its socket to appear. dir is set explicitly rather than inherited: the
// caller may run anywhere inside a workspace, and panes must open at its root.
func start(name string, env []string, dir string) (Session, error) {
	cmd := exec.Command("herdr", "--session", name, "server")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if err := detach(cmd); err != nil {
		return Session{}, fmt.Errorf("start herdr session %s: %w", name, err)
	}
	// Releasing the child stops it from becoming a zombie once fledge start
	// exits; herdr reparents to init and outlives us by design.
	if err := cmd.Process.Release(); err != nil {
		return Session{}, err
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s, found, err := Find(name)
		if err == nil && found && Up(s.SocketPath) {
			return s, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return Session{}, fmt.Errorf("herdr session %s did not come up", name)
}

// detach starts cmd in its own session so it survives the calling shell.
func detach(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devNull.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	return cmd.Start()
}
