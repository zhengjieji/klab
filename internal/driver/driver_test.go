package driver

import "testing"

func TestCapsCanRunArch(t *testing.T) {
	c := Caps{Arches: []string{"arm64"}}
	if !c.CanRunArch("arm64") {
		t.Fatal("expected arm64 supported")
	}
	if c.CanRunArch("x86_64") {
		t.Fatal("did not expect x86_64 supported")
	}
}
