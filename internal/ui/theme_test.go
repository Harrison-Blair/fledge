package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(value string) string { return ansiPattern.ReplaceAllString(value, "") }

func TestColorModeSet(t *testing.T) {
	for _, value := range []ColorMode{ColorAuto, ColorAlways, ColorNever} {
		var mode ColorMode
		if err := mode.Set(value.String()); err != nil || mode != value {
			t.Fatalf("Set(%q) = %q, %v", value, mode, err)
		}
	}
	var mode ColorMode
	if err := mode.Set("sometimes"); err == nil || !strings.Contains(err.Error(), "auto, always, or never") {
		t.Fatalf("invalid mode error = %v", err)
	}
}

func TestSelectProfileHonorsPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		mode          ColorMode
		json, noColor bool
		auto, always  termenv.Profile
		want          termenv.Profile
	}{
		{name: "auto tty", mode: ColorAuto, auto: termenv.TrueColor, want: termenv.TrueColor},
		{name: "auto redirected", mode: ColorAuto, auto: termenv.Ascii, want: termenv.Ascii},
		{name: "auto no color", mode: ColorAuto, noColor: true, auto: termenv.TrueColor, want: termenv.Ascii},
		{name: "always overrides no color", mode: ColorAlways, noColor: true, always: termenv.ANSI256, want: termenv.ANSI256},
		{name: "always permits pipes", mode: ColorAlways, always: termenv.Ascii, want: termenv.ANSI},
		{name: "never", mode: ColorNever, auto: termenv.TrueColor, always: termenv.TrueColor, want: termenv.Ascii},
		{name: "json overrides always", mode: ColorAlways, json: true, always: termenv.TrueColor, want: termenv.Ascii},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := selectProfile(test.mode, test.json, test.noColor, test.auto, test.always); got != test.want {
				t.Fatalf("profile = %s, want %s", got.Name(), test.want.Name())
			}
		})
	}
}

func TestThemeRendersSemanticRolesAndPreservesText(t *testing.T) {
	var output bytes.Buffer
	theme := NewThemeWithProfile(&output, termenv.TrueColor)
	styled := strings.Join([]string{
		theme.Accent("Title"),
		theme.Warning("Warning:"),
		theme.Error("Error [failed]"),
		theme.Status("working"),
		theme.Status("unknown"),
		theme.Status("blocked"),
		theme.Status("cancelled"),
	}, "|")
	if !strings.Contains(styled, "\x1b[") {
		t.Fatalf("styled output has no ANSI: %q", styled)
	}
	for name, sequence := range map[string]string{
		"accent":  "38;2;0;115;229",
		"warning": "38;2;146;112;34",
		"error":   "38;2;186;85;102",
	} {
		if !strings.Contains(styled, sequence) {
			t.Fatalf("%s palette color missing from %q", name, styled)
		}
	}
	if got, want := stripANSI(styled), "Title|Warning:|Error [failed]|working|unknown|blocked|cancelled"; got != want {
		t.Fatalf("stripped output = %q, want %q", got, want)
	}
	if strings.Contains(theme.Status("cancelled"), "\x1b[") {
		t.Fatalf("neutral state was styled: %q", theme.Status("cancelled"))
	}
}

func TestThemeModesAffectRedirectedOutput(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	for _, test := range []struct {
		name string
		mode ColorMode
		json bool
		ansi bool
	}{
		{name: "auto honors no color", mode: ColorAuto},
		{name: "always overrides no color", mode: ColorAlways, ansi: true},
		{name: "never", mode: ColorNever},
		{name: "json", mode: ColorAlways, json: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			rendered := NewTheme(&output, test.mode, test.json).Accent("label")
			if got := strings.Contains(rendered, "\x1b["); got != test.ansi {
				t.Fatalf("ANSI = %t, want %t: %q", got, test.ansi, rendered)
			}
		})
	}
}
