package cli

import "testing"

func TestPointerAndDisplay(t *testing.T) {
	if Pointer("") != nil || Display(nil) != "-" || Display(Pointer("")) != "-" {
		t.Fatal("empty values must render as -")
	}
	empty := ""
	if Display(&empty) != "-" {
		t.Fatal("an empty string must render as -")
	}
	if p := Pointer("w1"); p == nil || *p != "w1" || Display(p) != "w1" {
		t.Fatalf("Pointer/Display round trip: %v", p)
	}
}

func TestDisplayString(t *testing.T) {
	if got := DisplayString(""); got != "-" {
		t.Fatalf("DisplayString(\"\") = %q, want -", got)
	}
	if got := DisplayString("v1"); got != "v1" {
		t.Fatalf("DisplayString(v1) = %q, want v1", got)
	}
}
