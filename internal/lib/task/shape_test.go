package task

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestRecordShape(t *testing.T) {
	if ShapeStack != "stack" || ShapeFeature != "feature" || !slices.Equal(Shapes, []string{"stack", "feature"}) {
		t.Fatalf("%q %q %q", ShapeStack, ShapeFeature, Shapes)
	}
	var r Record
	if err := json.Unmarshal([]byte(`{"id":"0000000a"}`), &r); err != nil || r.Shape != "" {
		t.Fatalf("%v %+v", err, r)
	}
	b, err := json.Marshal(Record{Shape: ShapeStack})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal(b, &m)
	if m["shape"] != "stack" {
		t.Fatalf("%s", b)
	}
}
