// Package qemu implements the QEMU node driver: the day-1 workhorse that boots
// a custom kernel as a full VM with hardware acceleration (arm64 + KVM).
//
// The boot command line is produced by Argv as plain data *before* anything is
// exec'd, so it can be golden-snapshot-tested on hosted CI without a VM (PLAN
// §7.2). Boot/Exec/Stop (the live path) land in the Stage 1 boot slice.
package qemu

import (
	"context"
	"errors"
	"strconv"

	"github.com/zhengjieji/klab/internal/driver"
)

// Driver boots nodes via qemu-system-aarch64.
type Driver struct{}

// New returns a qemu driver.
func New() *Driver { return &Driver{} }

// Name identifies the driver in specs and diagnostics.
func (Driver) Name() string { return "qemu" }

// Capabilities: qemu boots a user-supplied kernel, needs /dev/kvm for the fast
// path, and (on an arm Mac) runs arm64 guests with hardware acceleration.
func (Driver) Capabilities() driver.Caps {
	return driver.Caps{CustomKernel: true, NeedsKVM: true, Arches: []string{"arm64"}}
}

// Argv builds the qemu-system-aarch64 command line for one node: direct
// `-kernel` boot, KVM acceleration, a 9p-exported rootfs, a serial console, and
// one virtio-net tap per link the node joins. It is pure and deterministic
// (stable flag order) so R1.4 can snapshot it as golden data.
func (Driver) Argv(spec driver.BootSpec) []string {
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
		"-nographic",
		"-no-reboot",
	}
	for i, tap := range spec.Taps {
		id := "net" + strconv.Itoa(i)
		argv = append(argv,
			"-netdev", "tap,id="+id+",ifname="+tap+",script=no,downscript=no",
			"-device", "virtio-net-pci,netdev="+id,
		)
	}
	return argv
}

// errNotImplemented marks the live boot path, which lands in the Stage 1 boot
// slice (F1.5–F1.7). Argv above is the tested Stage 1 deliverable (R1.4).
var errNotImplemented = errors.New("qemu: live boot not implemented yet (Stage 1 boot slice)")

// Boot is not implemented yet; see errNotImplemented.
func (Driver) Boot(context.Context, driver.BootSpec) (driver.Handle, error) {
	return "", errNotImplemented
}

// Exec is not implemented yet; see errNotImplemented.
func (Driver) Exec(context.Context, driver.Handle, []string) (driver.ExecResult, error) {
	return driver.ExecResult{}, errNotImplemented
}

// Stop is not implemented yet; see errNotImplemented.
func (Driver) Stop(context.Context, driver.Handle) error { return errNotImplemented }

// Compile-time check that the qemu driver satisfies the Driver interface.
var _ driver.Driver = (*Driver)(nil)
