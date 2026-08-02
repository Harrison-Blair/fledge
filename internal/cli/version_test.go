package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/Harrison-Blair/fledge/internal/buildinfo"
)

func TestVersionRequiresNeitherProjectNorHerdr(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute(context.Background(), []string{"version", "--json"}, nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.Version != buildinfo.Version() {
		t.Fatalf("envelope = %#v", envelope)
	}
}
