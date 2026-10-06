package task

import (
	"encoding/json"
	"testing"
)

func TestRecordWithoutUsageUnmarshalsNil(t *testing.T) {
	var r Record
	if err := json.Unmarshal([]byte(`{"id":"0000aaaa","status":"completed"}`), &r); err != nil || r.Usage != nil {
		t.Fatalf("%+v %v", r.Usage, err)
	}
}
