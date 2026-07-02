package driver

// TB is the subset of *testing.T the conformance suite needs, so the driver
// package does not import "testing" in non-test code. *testing.T satisfies it.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// Conformance runs the shared contract checks every Driver must satisfy without
// a host: a stable, non-empty name and self-consistent capabilities. Any driver
// (qemu now, firecracker later) is expected to pass it (R2.3). Live behavior
// (Boot/Exec/Stop) is exercised by each driver's own integration tests.
func Conformance(tb TB, d Driver) {
	tb.Helper()
	if d.Name() == "" {
		tb.Errorf("Driver.Name() must be non-empty")
	}
	c := d.Capabilities()
	if len(c.Arches) == 0 {
		tb.Errorf("driver %q: Capabilities().Arches must list at least one arch", d.Name())
	}
	for _, a := range c.Arches {
		if a == "" {
			tb.Errorf("driver %q: Capabilities().Arches contains an empty arch", d.Name())
		}
		if !c.CanRunArch(a) {
			tb.Errorf("driver %q: CanRunArch(%q) must be true for a listed arch", d.Name(), a)
		}
	}
	if c.CanRunArch("no-such-arch") {
		tb.Errorf("driver %q: CanRunArch must be false for an unlisted arch", d.Name())
	}
}
