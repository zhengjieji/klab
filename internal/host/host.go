// Package host reports whether the machine is ready to run klab: an
// Apple-silicon (M3+) macOS 15+ host, or a Linux host with KVM, carrying the
// tooling and an accelerated Linux box that exposes /dev/kvm.
//
// The verdict is a pure function of probed Facts (Evaluate), which is why it is
// exhaustively table-tested with injected environments (host_test.go). Probing
// the real machine — sysctl, sw_vers, limactl, ... — is the only impure part
// and lives in probe.go.
//
// scripts/doctor.sh mirrors these same rules for the pre-binary bootstrap path
// used by scripts/setup.sh, which must run before the klab binary exists. Keep
// the two in sync; this package is the source of truth for `klab doctor`.
package host

import (
	"fmt"
	"io"
)

// Level is the severity of a single readiness check.
type Level int

const (
	Pass Level = iota
	Warn
	Fail
)

func (l Level) marker() string {
	switch l {
	case Pass:
		return "✓"
	case Warn:
		return "!"
	default:
		return "✗"
	}
}

// Facts is the probed environment. Zero values mean unknown/absent: the probe
// fills what it can and Evaluate turns it into a verdict. Facts is the single
// injection point that makes the verdict logic unit-testable.
type Facts struct {
	OS string // "darwin" | "linux"

	// macOS host facts.
	ChipBrand  string // raw brand string, e.g. "Apple M4"; "" if unknown
	AppleGen   int    // Apple M generation (1, 2, 3, ...); 0 if not an Apple M chip
	Intel      bool   // an Intel Mac
	MacOSMajor int    // major macOS version; 0 if unknown

	MemGiB      int // total RAM, GiB
	FreeDiskGiB int // free disk for $HOME, GiB; -1 if unknown

	HasBrew    bool
	HasLimactl bool
	HasGo      bool

	// Accelerated host (macOS: the lima instance; Linux: this machine).
	LimaStatus string // "" (not created), "Running", "Stopped", ...
	KVMPresent bool   // /dev/kvm exists
	KVMOK      bool   // kvm-ok confirmed usable acceleration

	// Linux tooling.
	HasQEMU  bool
	HasClang bool
}

// Check is one readiness line in the report.
type Check struct {
	Level   Level
	Message string
	Note    string // optional actionable hint, shown indented
}

// Report is the outcome of evaluating Facts.
type Report struct {
	Checks []Check
	Fails  int
	Warns  int
}

// Ready reports whether the host can run klab (no hard failures; warnings ok).
func (r Report) Ready() bool { return r.Fails == 0 }

// ExitCode is 0 when ready, 1 otherwise — the process exit code for `klab doctor`.
func (r Report) ExitCode() int {
	if r.Fails > 0 {
		return 1
	}
	return 0
}

func (r *Report) add(l Level, msg, note string) {
	r.Checks = append(r.Checks, Check{Level: l, Message: msg, Note: note})
	switch l {
	case Warn:
		r.Warns++
	case Fail:
		r.Fails++
	}
}

func (r *Report) pass(msg string)       { r.add(Pass, msg, "") }
func (r *Report) warn(msg string)       { r.add(Warn, msg, "") }
func (r *Report) fail(msg, note string) { r.add(Fail, msg, note) }

// Evaluate turns probed Facts into a readiness Report. It is pure: no I/O, no
// globals, so identical Facts always yield an identical verdict.
func Evaluate(f Facts) Report {
	var r Report
	switch f.OS {
	case "darwin":
		evalDarwin(f, &r)
	case "linux":
		evalLinux(f, &r)
	default:
		r.fail("unsupported OS: "+f.OS, "klab supports macOS (Apple silicon) and Linux with KVM")
	}
	return r
}

func evalDarwin(f Facts, r *Report) {
	switch {
	case f.AppleGen >= 3:
		r.pass(fmt.Sprintf("chip: %s (M%d — nested virtualization supported)", f.ChipBrand, f.AppleGen))
	case f.AppleGen > 0:
		r.warn(fmt.Sprintf("chip: %s (M%d — no nested virtualization; in-VM KVM / microVM limited)", f.ChipBrand, f.AppleGen))
	case f.Intel:
		r.warn(fmt.Sprintf("chip: %s (Intel Mac — arm64 guests emulated; x86 native via KVM)", f.ChipBrand))
	default:
		r.warn(fmt.Sprintf("chip: %s (could not classify)", brandOrUnknown(f.ChipBrand)))
	}

	if f.MacOSMajor >= 15 {
		r.pass(fmt.Sprintf("macOS %d (>= 15, nested virt available)", f.MacOSMajor))
	} else {
		r.fail(fmt.Sprintf("macOS %d (need 15+ for nested virtualization)", f.MacOSMajor), "upgrade to macOS 15+ (Sequoia or later)")
	}

	switch {
	case f.MemGiB >= 16:
		r.pass(fmt.Sprintf("RAM %d GiB", f.MemGiB))
	case f.MemGiB >= 8:
		r.warn(fmt.Sprintf("RAM %d GiB (8 GiB: 1–2 VMs comfortable; clusters are a functional-only squeeze)", f.MemGiB))
	default:
		r.fail(fmt.Sprintf("RAM %d GiB (< 8 GiB is very tight)", f.MemGiB), "8 GiB is the practical minimum")
	}

	switch {
	case f.FreeDiskGiB < 0:
		r.warn("could not read free disk for $HOME")
	case f.FreeDiskGiB >= 60:
		r.pass(fmt.Sprintf("free disk %d GiB", f.FreeDiskGiB))
	default:
		r.warn(fmt.Sprintf("free disk %d GiB (kernel trees + rootfs want ~60 GiB)", f.FreeDiskGiB))
	}

	if f.HasBrew {
		r.pass("Homebrew")
	} else {
		r.fail("Homebrew not found", "install: https://brew.sh — or ./scripts/setup.sh")
	}
	if f.HasLimactl {
		r.pass("limactl")
	} else {
		r.fail("limactl not found", "brew install lima — or ./scripts/setup.sh")
	}
	if f.HasGo {
		r.pass("go")
	} else {
		r.fail("go not found", "brew install go — or ./scripts/setup.sh")
	}

	// The accelerated-host checks are only meaningful once limactl exists.
	if f.HasLimactl {
		evalLimaInstance(f, r)
	}
}

func evalLimaInstance(f Facts, r *Report) {
	switch f.LimaStatus {
	case "":
		r.warn("lima instance not created — run ./scripts/setup.sh")
		return
	case "Running":
		r.pass("lima instance running")
	default:
		r.warn(fmt.Sprintf("lima instance exists but is %q — start it with limactl start", f.LimaStatus))
		return
	}
	if f.KVMPresent {
		r.pass("/dev/kvm present inside the lima instance")
		if f.KVMOK {
			r.pass("KVM acceleration usable (kvm-ok)")
		} else {
			r.warn("kvm-ok did not confirm acceleration (install cpu-checker / re-provision)")
		}
	} else {
		r.fail("/dev/kvm missing inside the lima instance", "needs Apple M3+/macOS 15+ and nestedVirtualization: true")
	}
}

func evalLinux(f Facts, r *Report) {
	if f.KVMPresent {
		r.pass("/dev/kvm present")
		if f.KVMOK {
			r.pass("KVM acceleration usable")
		} else {
			r.warn("kvm-ok did not confirm (install cpu-checker)")
		}
	} else {
		r.fail("/dev/kvm missing", "enable KVM (or nested virtualization on a cloud VM)")
	}
	if f.HasGo {
		r.pass("go")
	} else {
		r.fail("go not found", "install Go 1.22+")
	}
	if f.HasQEMU {
		r.pass("qemu-system-aarch64")
	} else {
		r.fail("qemu-system-aarch64 not found", "install qemu")
	}
	if f.HasClang {
		r.pass("clang")
	} else {
		r.fail("clang not found", "install clang/llvm for kernel cross-compile")
	}
}

func brandOrUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// Fprint writes a human-readable report to w.
func (r Report) Fprint(w io.Writer) {
	fmt.Fprintln(w, "klab doctor")
	for _, c := range r.Checks {
		fmt.Fprintf(w, "  %s %s\n", c.Level.marker(), c.Message)
		if c.Note != "" {
			fmt.Fprintf(w, "    %s\n", c.Note)
		}
	}
	fmt.Fprintln(w)
	switch {
	case r.Fails > 0:
		fmt.Fprintf(w, "  NOT READY — %d issue(s), %d warning(s). Run ./scripts/setup.sh\n", r.Fails, r.Warns)
	case r.Warns > 0:
		fmt.Fprintf(w, "  READY with %d warning(s)\n", r.Warns)
	default:
		fmt.Fprintln(w, "  READY")
	}
}

// Run probes this machine, prints the report to w, and returns the process exit
// code (0 = ready, 1 = not ready).
func Run(w io.Writer) int {
	r := Evaluate(Probe())
	r.Fprint(w)
	return r.ExitCode()
}
