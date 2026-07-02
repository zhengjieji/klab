package driver

import (
	"context"
	"testing"
)

// recTB records how many Errorf calls the conformance suite makes.
type recTB struct{ errs int }

func (r *recTB) Helper()               {}
func (r *recTB) Errorf(string, ...any) { r.errs++ }

// fakeDriver is a configurable Driver for testing the conformance suite itself.
type fakeDriver struct {
	name string
	caps Caps
}

func (f fakeDriver) Name() string                                 { return f.name }
func (f fakeDriver) Capabilities() Caps                           { return f.caps }
func (fakeDriver) Boot(context.Context, BootSpec) (Handle, error) { return "", nil }
func (fakeDriver) Exec(context.Context, Handle, []string) (ExecResult, error) {
	return ExecResult{}, nil
}
func (fakeDriver) Stop(context.Context, Handle) error { return nil }

func TestConformance(t *testing.T) {
	// A well-formed driver passes cleanly.
	var r recTB
	Conformance(&r, fakeDriver{name: "good", caps: Caps{Arches: []string{"arm64"}}})
	if r.errs != 0 {
		t.Errorf("a well-formed driver reported %d conformance errors", r.errs)
	}

	// Each malformed driver is caught.
	bad := []fakeDriver{
		{name: "", caps: Caps{Arches: []string{"arm64"}}}, // no name
		{name: "x", caps: Caps{Arches: nil}},              // no arches
		{name: "y", caps: Caps{Arches: []string{""}}},     // empty arch
	}
	for _, d := range bad {
		var rb recTB
		Conformance(&rb, d)
		if rb.errs == 0 {
			t.Errorf("malformed driver %+v should have failed conformance", d)
		}
	}
}
