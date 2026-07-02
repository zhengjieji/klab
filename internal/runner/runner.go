// Package runner turns a topology into booted nodes: it resolves each node to a
// driver.BootSpec, checks the chosen driver's capabilities, and drives the
// driver's Boot/Exec/Stop. Stage 1 wires only the single-node qemu path; the
// N-node runner is Stage 2.
package runner

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zhengjieji/klab/internal/driver"
	"github.com/zhengjieji/klab/internal/topology"
)

// parseMem parses a memory size like "1G", "512M", or a bare MiB count ("2048")
// into MiB. An empty string is 0 (the driver applies its default).
func parseMem(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mult := 1
	switch s[len(s)-1] {
	case 'G', 'g':
		mult, s = 1024, s[:len(s)-1]
	case 'M', 'm':
		mult, s = 1, s[:len(s)-1]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("runner: invalid memory size %q", s)
	}
	return n * mult, nil
}

// resolveBootSpec builds a driver.BootSpec for a node from its topology entry
// plus the already-resolved kernel image and per-node rootfs/control paths. It
// is pure: path construction and cache lookup happen in the caller.
func resolveBootSpec(name string, n topology.Node, kernelImage, rootfs, rootfsRW string) (driver.BootSpec, error) {
	mem, err := parseMem(n.Mem)
	if err != nil {
		return driver.BootSpec{}, err
	}
	return driver.BootSpec{
		Name:     name,
		Kernel:   kernelImage,
		Rootfs:   rootfs,
		RootfsRW: rootfsRW,
		Arch:     n.Arch,
		CPU:      n.CPU,
		MemMiB:   mem,
	}, nil
}

// checkCaps rejects a node/driver mismatch before anything boots: a node that
// needs a custom kernel must not route to a driver that cannot boot one, and the
// driver must support the node's arch.
func checkCaps(driverName string, n topology.Node, caps driver.Caps) error {
	if n.Kernel != "" && !caps.CustomKernel {
		return fmt.Errorf("node %q needs a custom kernel but driver %q cannot boot one", n.Kernel, driverName)
	}
	if !caps.CanRunArch(n.Arch) {
		return fmt.Errorf("driver %q cannot run arch %q", driverName, n.Arch)
	}
	return nil
}
