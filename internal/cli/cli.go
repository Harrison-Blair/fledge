package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
	"github.com/Harrison-Blair/fledge/internal/fledge"
	"github.com/Harrison-Blair/fledge/internal/herdr"
	"github.com/Harrison-Blair/fledge/internal/project"
	"github.com/Harrison-Blair/fledge/internal/state"
	"github.com/Harrison-Blair/fledge/internal/ui"
	"github.com/spf13/cobra"
)

const schemaVersion = 1

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

type environment struct {
	in       io.Reader
	out      io.Writer
	errOut   io.Writer
	cwd      string
	stateDir string
	json     bool
	color    ui.ColorMode
	herdrBin string
	stdinTTY func() bool
	lookPath func(string) (string, error)
	getenv   func(string) string
	outTheme *ui.Theme
	errTheme *ui.Theme
}

type successEnvelope struct {
	SchemaVersion int  `json:"schema_version"`
	OK            bool `json:"ok"`
	Data          any  `json:"data"`
}

type errorEnvelope struct {
	SchemaVersion int       `json:"schema_version"`
	OK            bool      `json:"ok"`
	Error         errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Execute runs the CLI and returns a process exit code.
func Execute(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	env := &environment{
		in: in, out: out, errOut: errOut,
		color:    ui.ColorAuto,
		stdinTTY: func() bool { return isTerminalReader(in) },
		lookPath: exec.LookPath,
		getenv:   os.Getenv,
	}
	root := newRoot(env)
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	// Cobra can reject an unknown command before it binds persistent flags.
	// Preserve the requested error format in that early-parser path.
	if !env.json {
		env.json = jsonRequested(args)
	}
	if mode, requested := colorRequested(args); requested {
		env.color = mode
		env.errTheme = nil
	}
	code := 1
	body := errorBody{}
	var useErr *usageError
	var serviceErr *fledge.Error
	switch {
	case errors.As(err, &useErr):
		code = 2
		body = errorBody{Code: "usage_error", Message: useErr.Error()}
	case errors.As(err, &serviceErr):
		body = errorBody{Code: serviceErr.Code, Message: serviceErr.Message, Details: serviceErr.Details}
	default:
		// Command discovery and Cobra's argument/flag parser return ordinary
		// errors. Runtime paths wrap their failures in fledge.Error.
		code = 2
		body = errorBody{Code: "usage_error", Message: err.Error()}
	}
	if env.json {
		_ = json.NewEncoder(errOut).Encode(errorEnvelope{SchemaVersion: schemaVersion, OK: false, Error: body})
	} else {
		theme := env.stderrTheme()
		fmt.Fprintf(errOut, "%s: %s\n", theme.Error("Error ["+body.Code+"]"), body.Message)
	}
	return code
}

func jsonRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "--json", "-j", "--json=true", "-j=true":
			requested = true
		case "--json=false", "-j=false":
			requested = false
		}
	}
	return requested
}

func colorRequested(args []string) (ui.ColorMode, bool) {
	var mode ui.ColorMode
	requested := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		value := ""
		switch {
		case arg == "--color" || arg == "-c":
			if index+1 >= len(args) {
				continue
			}
			index++
			value = args[index]
		case strings.HasPrefix(arg, "--color="):
			value = strings.TrimPrefix(arg, "--color=")
		case strings.HasPrefix(arg, "-c="):
			value = strings.TrimPrefix(arg, "-c=")
		case strings.HasPrefix(arg, "-c") && len(arg) > 2:
			value = strings.TrimPrefix(arg, "-c")
		default:
			continue
		}
		var parsed ui.ColorMode
		if parsed.Set(value) == nil {
			mode, requested = parsed, true
		}
	}
	return mode, requested
}

func newRoot(env *environment) *cobra.Command {
	if env.color == "" {
		env.color = ui.ColorAuto
	}
	root := &cobra.Command{
		Use:           "fledge",
		Short:         "Manage project-scoped Herdr agent sessions",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().BoolVarP(&env.json, "json", "j", false, "emit machine-readable JSON")
	root.PersistentFlags().VarP(&env.color, "color", "c", "color output: auto, always, or never")
	root.PersistentFlags().StringVarP(&env.herdrBin, "herdr-bin", "H", "herdr", "path to the Herdr executable")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{message: err.Error()}
	})
	root.AddCommand(
		newInit(env),
		newStart(env),
		newStatus(env),
		newStop(env),
		newSessions(env),
		newAgent(env),
		newStopCleanup(env),
		newVersion(env),
	)
	return root
}

func newVersion(env *environment) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show Fledge build information",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			info := buildinfo.Current()
			return env.print(info, func(w io.Writer, theme *ui.Theme) {
				fmt.Fprintf(w, "%s %s\n", theme.Accent("fledge"), info.Version)
				if info.Revision != "" {
					fmt.Fprintf(w, "%s %s", theme.Accent("revision:"), info.Revision)
					if info.Modified {
						fmt.Fprint(w, " (modified)")
					}
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "%s %s\n", theme.Accent("go:"), info.GoVersion)
			})
		},
	}
}

func (e *environment) service(ctx context.Context) (*fledge.Service, error) {
	cwd, err := e.workingDirectory()
	if err != nil {
		return nil, err
	}
	info, err := discoverProject(cwd)
	if err != nil {
		return nil, err
	}
	store, err := e.newStore()
	if err != nil {
		return nil, err
	}
	binary := herdr.Binary{Path: e.herdrBin}
	resolution, err := fledge.ResolveSession(ctx, info.Root, binary)
	if err != nil {
		return nil, err
	}
	info.Session, info.SessionSource = resolution.Session, resolution.Source
	installed := resolution.Installed
	return &fledge.Service{
		Project: info, Binary: binary, Store: store, Installed: &installed, WorkspaceID: resolution.WorkspaceID,
		LaunchStopCleanup: launchDetachedStopCleanup,
		CallerPaneID:      strings.TrimSpace(e.getenvValue("HERDR_PANE_ID")),
	}, nil
}

func (e *environment) auditService() (*fledge.Service, error) {
	cwd, err := e.workingDirectory()
	if err != nil {
		return nil, err
	}
	info, err := discoverProject(cwd)
	if err != nil {
		return nil, err
	}
	info.Session, info.SessionSource = project.SessionName(info.Root), "derived"
	return &fledge.Service{Project: info}, nil
}

func (e *environment) messagingService() (*fledge.Service, error) {
	service, err := e.auditService()
	if err != nil {
		return nil, err
	}
	store, err := e.newStore()
	if err != nil {
		return nil, err
	}
	service.Store = store
	service.Binary = herdr.Binary{Path: e.herdrBin}
	service.CallerPaneID = strings.TrimSpace(e.getenvValue("HERDR_PANE_ID"))
	return service, nil
}

func discoverProject(cwd string) (project.Info, error) {
	info, err := project.Discover(cwd)
	if errors.Is(err, project.ErrNotInitialized) {
		return info, fledge.Wrap("project_not_initialized", err.Error(), err)
	}
	if err != nil {
		return info, fledge.Wrap("project_discovery_failed", err.Error(), err)
	}
	return info, nil
}

func (e *environment) newStore() (*state.Store, error) {
	store, err := state.New(e.stateDir)
	if err != nil {
		return nil, fledge.Wrap("state_unavailable", err.Error(), err)
	}
	return store, nil
}

func (e *environment) getenvValue(name string) string {
	if e.getenv != nil {
		return e.getenv(name)
	}
	return os.Getenv(name)
}

func (e *environment) workingDirectory() (string, error) {
	if e.cwd != "" {
		return e.cwd, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fledge.Wrap("project_discovery_failed", err.Error(), err)
	}
	return cwd, nil
}

func confirm(env *environment, prompt string) (bool, error) {
	fmt.Fprint(env.out, prompt)
	answer, err := bufio.NewReader(env.in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fledge.Wrap("input_failed", fmt.Sprintf("read confirmation: %v", err), err)
	}
	answer = strings.TrimSpace(answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func (e *environment) print(data any, human func(io.Writer, *ui.Theme)) error {
	if e.json {
		return json.NewEncoder(e.out).Encode(successEnvelope{SchemaVersion: schemaVersion, OK: true, Data: data})
	}
	human(e.out, e.stdoutTheme())
	return nil
}

func (e *environment) stdoutTheme() *ui.Theme {
	if e.outTheme == nil {
		e.outTheme = ui.NewTheme(e.out, e.colorMode(), e.json)
	}
	return e.outTheme
}

func (e *environment) stderrTheme() *ui.Theme {
	if e.errTheme == nil {
		e.errTheme = ui.NewTheme(e.errOut, e.colorMode(), e.json)
	}
	return e.errTheme
}

func (e *environment) colorMode() ui.ColorMode {
	if e.color == "" {
		return ui.ColorAuto
	}
	return e.color
}

func noArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return usage("this command accepts no arguments")
	}
	return nil
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != count {
			return usage(fmt.Sprintf("%s requires exactly %d argument(s)", cmd.CommandPath(), count))
		}
		return nil
	}
}

func usage(message string) error { return &usageError{message: message} }
