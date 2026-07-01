package host

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// limaInstance is the lima instance klab manages; overridable for parity with
// scripts/doctor.sh (KLAB_LIMA_INSTANCE).
func limaInstance() string {
	if v := os.Getenv("KLAB_LIMA_INSTANCE"); v != "" {
		return v
	}
	return "klab"
}

var appleGenRE = regexp.MustCompile(`Apple M(\d+)`)

// Probe inspects the real machine and returns Facts. This is the only impure
// part of the package: Evaluate does the rest and is unit-tested against
// injected Facts, so nothing here needs a test host.
func Probe() Facts {
	f := Facts{OS: runtime.GOOS, FreeDiskGiB: -1}
	switch runtime.GOOS {
	case "darwin":
		probeDarwin(&f)
	case "linux":
		probeLinux(&f)
	}
	return f
}

func probeDarwin(f *Facts) {
	f.ChipBrand = out("sysctl", "-n", "machdep.cpu.brand_string")
	if m := appleGenRE.FindStringSubmatch(f.ChipBrand); m != nil {
		f.AppleGen, _ = strconv.Atoi(m[1])
	}
	f.Intel = strings.Contains(f.ChipBrand, "Intel")

	if v := out("sw_vers", "-productVersion"); v != "" {
		f.MacOSMajor, _ = strconv.Atoi(strings.SplitN(v, ".", 2)[0])
	}
	if b := out("sysctl", "-n", "hw.memsize"); b != "" {
		if n, err := strconv.ParseInt(b, 10, 64); err == nil {
			f.MemGiB = int(n / (1 << 30))
		}
	}
	f.FreeDiskGiB = freeDiskGiB(home())

	f.HasBrew = have("brew")
	f.HasLimactl = have("limactl")
	f.HasGo = have("go")

	if f.HasLimactl {
		inst := limaInstance()
		f.LimaStatus = out("limactl", "list", inst, "--format", "{{.Status}}")
		if f.LimaStatus == "Running" {
			f.KVMPresent = run("limactl", "shell", inst, "--", "test", "-e", "/dev/kvm")
			if f.KVMPresent {
				f.KVMOK = run("limactl", "shell", inst, "--", "sh", "-c",
					"command -v kvm-ok >/dev/null 2>&1 && sudo -n kvm-ok")
			}
		}
	}
}

func probeLinux(f *Facts) {
	_, err := os.Stat("/dev/kvm")
	f.KVMPresent = err == nil
	if f.KVMPresent {
		f.KVMOK = run("sh", "-c", "command -v kvm-ok >/dev/null 2>&1 && sudo -n kvm-ok")
	}
	f.HasGo = have("go")
	f.HasQEMU = have("qemu-system-aarch64")
	f.HasClang = have("clang")
	f.FreeDiskGiB = freeDiskGiB(home())
}

// freeDiskGiB returns free GiB for dir, or -1 if it cannot be read.
func freeDiskGiB(dir string) int {
	// `df -g` reports 1-GiB blocks on both macOS and Linux (GNU coreutils).
	o := out("df", "-g", dir)
	lines := strings.Split(strings.TrimSpace(o), "\n")
	if len(lines) < 2 {
		return -1
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return -1
	}
	n, err := strconv.Atoi(fields[3]) // Available
	if err != nil {
		return -1
	}
	return n
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func have(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}

// out runs a command and returns trimmed stdout, or "" on any error.
func out(name string, args ...string) string {
	b, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// run reports whether a command exited zero.
func run(name string, args ...string) bool {
	return exec.Command(name, args...).Run() == nil
}
