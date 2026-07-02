// Package qemu implements the QEMU node driver: the day-1 workhorse that boots
// a custom kernel as a full VM with hardware acceleration (arm64 + KVM).
//
// The boot command line is produced by Argv as plain data *before* anything is
// exec'd, so it can be golden-snapshot-tested on hosted CI without a VM (PLAN
// §7.2). Boot/Exec/Stop (the live path) land in the Stage 1 boot slice.
package qemu

import (
	"strconv"

	"github.com/zhengjieji/klab/internal/driver"
	kexec "github.com/zhengjieji/klab/internal/exec"
)

// Driver boots nodes via qemu-system-aarch64. Runner is the seam into the lima
// VM (where qemu and /dev/kvm live); a nil Runner defaults to a LimaRunner on
// the "klab" instance.
type Driver struct {
	Runner kexec.Runner
}

// New returns a qemu driver.
func New() *Driver { return &Driver{} }

// Name identifies the driver in specs and diagnostics.
func (Driver) Name() string { return "qemu" }

// Capabilities: qemu boots a user-supplied kernel, needs /dev/kvm for the fast
// path, and (on an arm Mac) runs arm64 guests with hardware acceleration.
func (Driver) Capabilities() driver.Caps {
	return driver.Caps{CustomKernel: true, NeedsKVM: true, Arches: []string{"arm64"}}
}

// Argv builds the pure boot command line for one node: direct `-kernel` boot,
// KVM acceleration, a 9p-exported rootfs, a serial console, and one virtio-net
// tap per link. Deterministic (stable flag order) so R1.4 can snapshot it.
func (Driver) Argv(spec driver.BootSpec) []string {
	return bootArgv(spec, "", "")
}

// bootArgv is Argv's implementation, parameterized for the live boot path.
// extraCmdline is appended to `-append` (e.g. "init=/sbin/klab-init"); when
// rwPath is non-empty a second 9p device (mount_tag=klabrw) exports it as the
// node's control/scratch channel. Both the plain Argv and the live boot line are
// golden-snapshot-tested, so this stays pure and its flag order stable.
func bootArgv(spec driver.BootSpec, extraCmdline, rwPath string) []string {
	cpu := spec.CPU
	if cpu < 1 {
		cpu = 1
	}
	mem := spec.MemMiB
	if mem < 1 {
		mem = 512
	}
	cmdline := "console=ttyAMA0 root=rootfs rootfstype=9p " +
		"rootflags=trans=virtio,version=9p2000.L rw"
	if extraCmdline != "" {
		cmdline += " " + extraCmdline
	}

	argv := []string{
		"qemu-system-aarch64",
		"-machine", "virt,gic-version=max",
		"-accel", "kvm",
		"-cpu", "host",
		"-smp", strconv.Itoa(cpu),
		"-m", strconv.Itoa(mem),
		"-kernel", spec.Kernel,
		"-append", cmdline,
		"-fsdev", "local,id=rootdev,path=" + spec.Rootfs + ",security_model=none",
		"-device", "virtio-9p-pci,fsdev=rootdev,mount_tag=rootfs",
	}
	if rwPath != "" {
		argv = append(argv,
			"-fsdev", "local,id=klabrw,path="+rwPath+",security_model=none",
			"-device", "virtio-9p-pci,fsdev=klabrw,mount_tag=klabrw",
		)
	}
	argv = append(argv, "-nographic", "-no-reboot")
	for i, tap := range spec.Taps {
		id := "net" + strconv.Itoa(i)
		argv = append(argv,
			"-netdev", "tap,id="+id+",ifname="+tap+",script=no,downscript=no",
			"-device", "virtio-net-pci,netdev="+id,
		)
	}
	return argv
}

// Compile-time check that the qemu driver satisfies the Driver interface
// (Boot/Exec/Stop are implemented in live.go).
var _ driver.Driver = (*Driver)(nil)
