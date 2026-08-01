// Package ui contains the shared presentation rules for Fledge's human output.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Harrison-Blair/fledge/internal/processenv"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ColorMode controls when Fledge adds ANSI styling to human-readable output.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

func (m ColorMode) String() string { return string(m) }

// Set implements pflag.Value and rejects unsupported color modes during Cobra
// flag parsing.
func (m *ColorMode) Set(value string) error {
	switch ColorMode(value) {
	case ColorAuto, ColorAlways, ColorNever:
		*m = ColorMode(value)
		return nil
	default:
		return fmt.Errorf("invalid color mode %q (must be auto, always, or never)", value)
	}
}

func (m ColorMode) Type() string { return "color" }

// Role describes the semantic meaning of a piece of output.
type Role uint8

const (
	RoleNeutral Role = iota
	RoleAccent
	RoleWarning
	RoleError
)

const (
	accentColor  = "#0073E5"
	warningColor = "#927022"
	errorColor   = "#BA5566"
)

// Theme holds renderer-bound styles. A Theme never probes the terminal's
// background color; the same dual-contrast palette is used everywhere.
type Theme struct {
	renderer *lipgloss.Renderer
	enabled  bool

	plainStyle   lipgloss.Style
	accentStyle  lipgloss.Style
	warningStyle lipgloss.Style
	errorStyle   lipgloss.Style
}

// NewTheme creates a theme for one output destination. JSON always disables
// styling, regardless of the requested mode.
func NewTheme(w io.Writer, mode ColorMode, jsonOutput bool) *Theme {
	profile := profileFor(w, mode, jsonOutput)
	return newTheme(w, profile)
}

// NewThemeWithProfile creates a deterministic theme for tests and other
// presentation contexts that already know the terminal profile.
func NewThemeWithProfile(w io.Writer, profile termenv.Profile) *Theme {
	return newTheme(w, profile)
}

func newTheme(w io.Writer, profile termenv.Profile) *Theme {
	renderer := lipgloss.NewRenderer(w)
	renderer.SetColorProfile(profile)
	theme := &Theme{
		renderer: renderer,
		enabled:  profile != termenv.Ascii,
	}
	theme.plainStyle = renderer.NewStyle()
	if theme.enabled {
		theme.accentStyle = renderer.NewStyle().Bold(true).Foreground(lipgloss.Color(accentColor))
		theme.warningStyle = renderer.NewStyle().Bold(true).Foreground(lipgloss.Color(warningColor))
		theme.errorStyle = renderer.NewStyle().Bold(true).Foreground(lipgloss.Color(errorColor))
	} else {
		// Empty renderer-bound styles are intentional: unlike an Ascii color
		// profile, they suppress bold and every other ANSI decoration too.
		theme.accentStyle = theme.plainStyle
		theme.warningStyle = theme.plainStyle
		theme.errorStyle = theme.plainStyle
	}
	return theme
}

func profileFor(w io.Writer, mode ColorMode, jsonOutput bool) termenv.Profile {
	_, noColor := os.LookupEnv("NO_COLOR")
	// Auto is intentionally stricter than CLICOLOR_FORCE: the Fledge
	// contract emits color only when this destination is actually a TTY.
	autoProfile := termenv.NewOutput(w).ColorProfile()
	alwaysProfile := termenv.NewOutput(w,
		termenv.WithTTY(true), termenv.WithEnvironment(withoutNoColor{})).ColorProfile()
	return selectProfile(mode, jsonOutput, noColor, autoProfile, alwaysProfile)
}

func selectProfile(
	mode ColorMode,
	jsonOutput bool,
	noColor bool,
	autoProfile termenv.Profile,
	alwaysProfile termenv.Profile,
) termenv.Profile {
	if jsonOutput || mode == ColorNever || mode == ColorAuto && noColor {
		return termenv.Ascii
	}
	if mode == ColorAuto {
		return autoProfile
	}
	if alwaysProfile == termenv.Ascii {
		return termenv.ANSI
	}
	return alwaysProfile
}

// withoutNoColor delegates to the process environment except for NO_COLOR.
// It is used only for an explicit --color always override.
type withoutNoColor struct{}

func (withoutNoColor) Environ() []string {
	return processenv.WithoutNoColor(os.Environ())
}

func (withoutNoColor) Getenv(name string) string {
	if name == "NO_COLOR" {
		return ""
	}
	return os.Getenv(name)
}

func (t *Theme) Enabled() bool { return t != nil && t.enabled }

func (t *Theme) Plain() lipgloss.Style {
	if t == nil {
		return lipgloss.NewStyle()
	}
	return t.plainStyle
}

func (t *Theme) Accent(value string) string  { return t.render(RoleAccent, value) }
func (t *Theme) Warning(value string) string { return t.render(RoleWarning, value) }
func (t *Theme) Error(value string) string   { return t.render(RoleError, value) }

func (t *Theme) Style(role Role) lipgloss.Style {
	if t == nil {
		return lipgloss.NewStyle()
	}
	switch role {
	case RoleAccent:
		return t.accentStyle
	case RoleWarning:
		return t.warningStyle
	case RoleError:
		return t.errorStyle
	default:
		return t.plainStyle
	}
}

func (t *Theme) render(role Role, value string) string {
	return t.Style(role).Render(value)
}

// StatusRole maps lifecycle, server, run, delivery, and message states onto
// the shared semantic palette. Unrecognized and stopped/cancelled states stay
// neutral.
func StatusRole(status string) Role {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "unknown", "uncertain":
		return RoleWarning
	case "blocked", "failed":
		return RoleError
	case "idle", "working", "done", "running", "active", "queued", "awaiting_ack", "acknowledged", "delivered", "created", "attempted", "injected":
		return RoleAccent
	default:
		return RoleNeutral
	}
}

func (t *Theme) Status(status string) string {
	return t.render(StatusRole(status), status)
}
