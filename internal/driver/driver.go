// Package driver defines the node-driver abstraction. A driver knows how to
// boot, exec into, and stop one node of a topology on a given backend (QEMU
// full VM, Firecracker microVM, a remote cloud host, ...).
//
// The topology runner selects a driver per node and checks its Capabilities
// against what the node requires, so impossible combinations (e.g. a custom
// kernel on a shared-kernel backend) are rejected before anything boots.
package driver

import "context"

// Caps describes what a driver can do.
type Caps struct {
	// CustomKernel is true if the driver can boot a user-supplied kernel image.
	// Custom-kernel work requires this; the container backend does not have it.
	CustomKernel bool
	// NeedsKVM is true if the driver requires /dev/kvm (on Apple silicon this
	// means nested virtualization inside the host VM, M3+/macOS 15+).
	NeedsKVM bool
	// Arches lists guest architectures this driver can run, e.g. ["arm64"].
	Arches []string
}

// CanRunArch reports whether the driver supports the given guest arch.
func (c Caps) CanRunArch(arch string) bool {
	for _, a := range c.Arches {
		if a == arch {
			return true
		}
	}
	return false
}

// Handle is an opaque reference to a running node.
type Handle string

// BootSpec is the resolved, driver-agnostic description of one node to boot.
type BootSpec struct {
	Name     string
	Kernel   string // path to a built kernel image (Stage 1 artifact)
	Rootfs   string // path to the node's rootfs, exported to the guest over 9p
	RootfsRW string // control/scratch dir; exported as a 2nd 9p device (empty = none)
	Arch     string
	CPU      int
	MemMiB   int
	Nics     []NIC // one per link the node joins
}

// NIC is a node's network interface: a host tap device and a unique guest MAC.
// A unique MAC per node is required — two nodes on one L2 segment sharing a MAC
// (e.g. qemu's default 52:54:00:12:34:56) cannot communicate.
type NIC struct {
	Tap string
	MAC string
}

// ExecResult is the outcome of a command run inside a node.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Driver is the backend interface implemented by qemu, firecracker, etc.
// Stage 1 lands the qemu implementation; Stage 4 lands firecracker. New drivers
// must pass the shared conformance suite (see PLAN §9 Stage 2).
type Driver interface {
	Name() string
	Capabilities() Caps
	Boot(ctx context.Context, spec BootSpec) (Handle, error)
	Exec(ctx context.Context, h Handle, argv []string) (ExecResult, error)
	Stop(ctx context.Context, h Handle) error
}
