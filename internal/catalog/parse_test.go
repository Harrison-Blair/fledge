package catalog

import (
	"reflect"
	"testing"
)

// piSample is real pi --list-models output, trailing column padding included.
const piSample = `provider      model                   context  max-out  thinking  images
openai-codex  gpt-5.3-codex-spark     128K     128K     yes       no    
openai-codex  gpt-5.4                 272K     128K     yes       yes   
openai-codex  gpt-5.5                 272K     128K     yes       yes   
opencode      big-pickle              200K     32K      yes       no    
opencode      claude-fable-5          1M       128K     yes       yes   
opencode      claude-opus-4-8         1M       128K     yes       yes   
opencode-go   glm-5                   128K     64K      yes       yes
`

func TestParsePiTable(t *testing.T) {
	got := parsePiTable(piSample)
	want := []piRow{
		{provider: "openai-codex", model: "gpt-5.3-codex-spark"},
		{provider: "openai-codex", model: "gpt-5.4"},
		{provider: "openai-codex", model: "gpt-5.5"},
		{provider: "opencode", model: "big-pickle"},
		{provider: "opencode", model: "claude-fable-5"},
		{provider: "opencode", model: "claude-opus-4-8"},
		{provider: "opencode-go", model: "glm-5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsePiTable = %#v, want %#v", got, want)
	}
}

func TestParsePiTableSkipsUnusableLines(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []piRow
	}{
		{name: "empty", out: ""},
		{name: "header only", out: "provider  model  context\n"},
		{name: "blank lines", out: "\n   \n"},
		{name: "single field", out: "openai-codex\n"},
		{name: "rows without header", out: "opencode  claude-opus-4-8  1M\n"},
		{name: "case-sensitive header", out: "Provider  model\nopencode  claude-opus-4-8  1M\n"},
		{
			name: "row without trailing newline",
			out:  "provider model\nopencode  claude-opus-4-8  1M",
			want: []piRow{{provider: "opencode", model: "claude-opus-4-8"}},
		},
		{
			name: "ignores prose before extended header",
			out:  "Available models\nrun pi with a model\nprovider model context extra\nopencode claude-opus-4-8 1M yes\n",
			want: []piRow{{provider: "opencode", model: "claude-opus-4-8"}},
		},
		{
			name: "skips blanks short lines and repeated headers",
			out:  "provider model context\n\nopencode\nprovider model context\nopencode big-pickle 200K\n",
			want: []piRow{{provider: "opencode", model: "big-pickle"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parsePiTable(tc.out); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parsePiTable = %#v, want %#v", got, tc.want)
			}
		})
	}
}
