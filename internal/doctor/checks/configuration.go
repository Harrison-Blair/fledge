package checks

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Harrison-Blair/fledge/internal/doctor/report"
)

// Environment reads process environment state without touching globals, so
// tests can drive each configuration branch deterministically.
type Environment struct {
	Getenv func(string) string
	Stat   func(string) (fs.FileInfo, error)
	Getwd  func() (string, error)
}

// LocalEnvironment reads the real process environment and filesystem.
func LocalEnvironment() Environment {
	return Environment{Getenv: os.Getenv, Stat: os.Stat, Getwd: os.Getwd}
}

// Configuration inspects the Herdr environment variables and socket mode.
func Configuration(e Environment) report.Check {
	herdrEnv := e.Getenv("HERDR_ENV")
	socketPath := e.Getenv("HERDR_SOCKET_PATH")
	paneID := e.Getenv("HERDR_PANE_ID")
	session := e.Getenv("HERDR_SESSION")
	cwd, _ := e.Getwd()

	status := report.OK
	if herdrEnv != "1" {
		status = report.Fail
	}

	socketStatus, mode, socketDetail := socketStatus(e, socketPath)
	status = report.Worst(status, socketStatus)

	if paneID == "" {
		status = report.Worst(status, report.Warn)
	}

	data := report.ConfigData{HerdrEnv: herdrEnv, SocketPath: socketPath, SocketMode: mode, PaneID: paneID, Session: session, Cwd: cwd, SocketField: socketDetail}
	detail := fmt.Sprintf("HERDR_ENV=%s socket=%s pane=%s session=%s cwd=%s", dash(herdrEnv), socketDetail, dash(paneID), dash(session), dash(cwd))
	return report.Check{Name: "configuration", Status: status, Detail: detail, Data: data}
}

// socketStatus stats the socket path and reports its status, octal mode, and a
// detail fragment describing the path.
func socketStatus(e Environment, path string) (report.Status, string, string) {
	if path == "" {
		return report.Fail, "", "unset"
	}
	info, err := e.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return report.Fail, "", path + " (missing)"
		}
		// A permission or transient stat error leaves the mode unknown, not gone.
		return report.Warn, "", fmt.Sprintf("%s (mode unknown: %v)", path, err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		return report.Fail, "", path + " (not a socket)"
	}
	mode := fmt.Sprintf("%04o", info.Mode().Perm())
	if info.Mode().Perm() == 0o600 {
		return report.OK, mode, fmt.Sprintf("%s (%s)", path, mode)
	}
	return report.Warn, mode, fmt.Sprintf("%s (mode %s)", path, mode)
}
