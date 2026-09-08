package herdr

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRenderArg(t *testing.T) {
	long := strings.Repeat("s", maxVerbatimBytes+1)
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "plain", arg: "w1:p1", want: "w1:p1"},
		{name: "path-like", arg: "/tmp/x.toml", want: "/tmp/x.toml"},
		{name: "empty", arg: "", want: `""`},
		{name: "space", arg: "docs ws", want: `"docs ws"`},
		{name: "unicode", arg: "héllo", want: `"héllo"`},
		{name: "zero width", arg: "a\u200bb", want: `"a\u200bb"`},
		{name: "control", arg: "a\x01b\n", want: `"a\x01b\n"`},
		{name: "invalid utf-8", arg: "\xff\xfe", want: `"\xff\xfe"`},
		{name: "quote and backslash", arg: `a"b\c`, want: `"a\"b\\c"`},
		{name: "angle bracket", arg: "<7", want: `"<7"`},
		{name: "at limit", arg: strings.Repeat("s", maxVerbatimBytes), want: strings.Repeat("s", maxVerbatimBytes)},
		{name: "over limit", arg: long, want: "<49 bytes>"},
		{name: "over limit unicode", arg: strings.Repeat("é", 30), want: "<60 bytes>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderArg(tc.arg); got != tc.want {
				t.Fatalf("renderArg(%q) = %q, want %q", tc.arg, got, tc.want)
			}
		})
	}
}

func TestRenderOperation(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		got := renderOperation([]string{"--session", "managed", "agent", "start", "reviewer", "--kind", "claude", "--", "--model", "opus"})
		if want := "herdr --session managed agent start reviewer --kind claude -- --model opus"; got != want {
			t.Fatalf("renderOperation = %q, want %q", got, want)
		}
	})

	t.Run("no arguments", func(t *testing.T) {
		if got := renderOperation(nil); got != "herdr" {
			t.Fatalf("renderOperation(nil) = %q", got)
		}
	})

	t.Run("argument count cap", func(t *testing.T) {
		var argv []string
		for i := 0; i < 100; i++ {
			argv = append(argv, "a"+strconv.Itoa(i))
		}
		got := renderOperation(argv)
		want := "herdr " + strings.Join(argv[:maxRenderedArgs], " ") + " …+84 args"
		if got != want {
			t.Fatalf("renderOperation = %q, want %q", got, want)
		}
	})

	t.Run("huge value summarized without prefix", func(t *testing.T) {
		huge := strings.Repeat("p", 100000)
		got := renderOperation([]string{"agent", "prompt", "w1:p1", huge})
		if want := "herdr agent prompt w1:p1 <100000 bytes>"; got != want {
			t.Fatalf("renderOperation = %q, want %q", got, want)
		}
	})

	t.Run("total byte cap", func(t *testing.T) {
		value := strings.Repeat("v", maxVerbatimBytes)
		var argv []string
		for i := 0; i < maxRenderedArgs; i++ {
			argv = append(argv, value)
		}
		got := renderOperation(argv)
		if len(got) > maxOperationBytes {
			t.Fatalf("rendered %d bytes: %q", len(got), got)
		}
		if !strings.HasSuffix(got, " args") || !strings.Contains(got, "…+") {
			t.Fatalf("rendered operation lacks omission marker: %q", got)
		}
		rendered := strings.Count(got, value)
		omitted, err := strconv.Atoi(strings.TrimSuffix(got[strings.LastIndex(got, "+")+1:], " args"))
		if err != nil || rendered+omitted != maxRenderedArgs {
			t.Fatalf("rendered %d and omitted %d (%v): %q", rendered, omitted, err, got)
		}
	})

	t.Run("never exceeds the cap", func(t *testing.T) {
		for count := 0; count < 40; count++ {
			for size := 0; size <= maxVerbatimBytes+1; size += 7 {
				var argv []string
				for i := 0; i < count; i++ {
					argv = append(argv, strings.Repeat("\xff", size))
				}
				if got := renderOperation(argv); len(got) > maxOperationBytes {
					t.Fatalf("count %d size %d rendered %d bytes", count, size, len(got))
				}
			}
		}
	})
}

func TestRenderText(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		limit int
		want  string
	}{
		{name: "plain", text: "unknown option: --nope", limit: 64, want: "unknown option: --nope"},
		{name: "unicode kept", text: "pane «w1:p1» héllo", limit: 64, want: "pane «w1:p1» héllo"},
		{name: "newlines and tabs", text: "a\nb\tc\rd", limit: 64, want: `a\nb\tc\rd`},
		{name: "controls", text: "a\x00b\x1b[0m", limit: 64, want: `a\u0000b\u001b[0m`},
		{name: "backslash", text: `C:\x`, limit: 64, want: `C:\\x`},
		{name: "invalid utf-8", text: "a\xffb", limit: 64, want: `a\xffb`},
		{name: "zero width", text: "a\u200bb", limit: 64, want: `a\u200bb`},
		{name: "astral format char", text: "a\U000E0001b", limit: 64, want: `a\U000e0001b`},
		{name: "truncated", text: strings.Repeat("x", 100), limit: 32, want: strings.Repeat("x", 19) + " …+81 bytes"},
		{name: "empty", text: "", limit: 8, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderText(tc.text, tc.limit)
			if got != tc.want {
				t.Fatalf("renderText(%q, %d) = %q, want %q", tc.text, tc.limit, got, tc.want)
			}
			if len(got) > tc.limit {
				t.Fatalf("rendered %d bytes over limit %d", len(got), tc.limit)
			}
		})
	}

	t.Run("cuts on rune boundaries", func(t *testing.T) {
		text := strings.Repeat("é", 200)
		for limit := 16; limit < 64; limit++ {
			got := renderText(text, limit)
			if !utf8.ValidString(got) || len(got) > limit {
				t.Fatalf("limit %d rendered %q", limit, got)
			}
		}
	})

	t.Run("huge", func(t *testing.T) {
		got := renderOutput(strings.Repeat("m", 1<<20))
		if len(got) > maxOutputBytes || !strings.HasSuffix(got, " …+"+strconv.Itoa(1<<20-len(got)+len(" …+1048075 bytes"))+" bytes") {
			t.Fatalf("renderOutput of 1 MiB = %d bytes: %q", len(got), got)
		}
	})
}

func TestCommandErrorBoundsOutput(t *testing.T) {
	fakeHerdr(t, `printf '%s' "$HERDR_FAKE_STDERR" >&2; exit 1`)
	t.Setenv("HERDR_FAKE_STDERR", "boom\n"+strings.Repeat("tail", 10000))

	_, err := New(nil, nil, nil).Agents(context.Background())
	if err == nil {
		t.Fatal("Agents succeeded")
	}
	text := err.Error()
	if len(text) > maxOutputBytes+128 || !strings.HasPrefix(text, "herdr agent list: exit status 1: boom\\ntail") || !strings.Contains(text, "…+") {
		t.Fatalf("error text (%d bytes) = %q", len(text), text)
	}
}

func TestDecodeErrorBoundsFieldsAndKeepsCodes(t *testing.T) {
	message := strings.Repeat("m", 100000)
	fakeHerdr(t, `printf '%s' "$HERDR_FAKE_STDERR" >&2; exit 1`)
	t.Setenv("HERDR_FAKE_STDERR", `{"error":{"code":"`+strings.Repeat("c", 1000)+`","message":"`+message+`\n\u0001"},"id":"cli:agent:list"}`)

	_, err := New(nil, nil, nil).Agents(context.Background())
	var reported *Error
	if !errors.As(err, &reported) {
		t.Fatalf("Agents error = %v, want *Error", err)
	}
	if len(reported.Code) > maxCodeBytes || !strings.HasSuffix(reported.Code, " bytes") {
		t.Fatalf("Code = %q", reported.Code)
	}
	if len(reported.Message) > maxOutputBytes || !strings.HasPrefix(reported.Message, "mmm") || !strings.Contains(reported.Message, "…+") {
		t.Fatalf("Message = %q", reported.Message)
	}
	if reported.Operation != "herdr agent list" {
		t.Fatalf("Operation = %q", reported.Operation)
	}

	t.Setenv("HERDR_FAKE_STDERR", `{"error":{"code":"pane_not_found","message":"pane\tw1:p1 \\ not found"}}`)
	_, err = New(nil, nil, nil).Agents(context.Background())
	if !errors.As(err, &reported) || reported.Code != "pane_not_found" || reported.Message != `pane\tw1:p1 \\ not found` {
		t.Fatalf("error = %#v", err)
	}
}

func TestSessionOperationsRenderBounded(t *testing.T) {
	name := strings.Repeat("n", 1000)
	fakeHerdr(t, `exit 1`)

	err := New(nil, nil, nil).Stop(context.Background(), name)
	if err == nil || !strings.HasPrefix(err.Error(), "herdr session stop <1000 bytes> --json: ") {
		t.Fatalf("Stop error = %v", err)
	}
	err = New(nil, nil, nil).Launch(context.Background(), t.TempDir(), "bad name")
	if err == nil || !strings.HasPrefix(err.Error(), `herdr --session "bad name": `) {
		t.Fatalf("Launch error = %v", err)
	}
}
