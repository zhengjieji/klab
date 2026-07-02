package qemu

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zhengjieji/klab/internal/driver"
)

func TestNameAndCaps(t *testing.T) {
	d := New()
	if d.Name() != "qemu" {
		t.Errorf("Name() = %q, want qemu", d.Name())
	}
	c := d.Capabilities()
	if !c.CustomKernel || !c.NeedsKVM || !c.CanRunArch("arm64") || c.CanRunArch("x86_64") {
		t.Errorf("unexpected caps: %+v", c)
	}
}

// TestArgvGolden pins the generated boot command line so kernel path, accel,
// 9p rootfs, console, and per-tap net flags cannot silently regress (R1.4).
func TestArgvGolden(t *testing.T) {
	tests := []struct {
		name string
		spec driver.BootSpec
		want []string
	}{
		{
			name: "single node, no links",
			spec: driver.BootSpec{
				Name: "vm1", Kernel: "/cache/abc123/Image", Rootfs: "/rootfs/vm1",
				Arch: "arm64", CPU: 2, MemMiB: 1024,
			},
			want: []string{
				"qemu-system-aarch64",
				"-machine", "virt,gic-version=max",
				"-accel", "kvm",
				"-cpu", "host",
				"-smp", "2",
				"-m", "1024",
				"-kernel", "/cache/abc123/Image",
				"-append", "console=ttyAMA0 root=rootfs rootfstype=9p rootflags=trans=virtio,version=9p2000.L rw",
				"-fsdev", "local,id=rootdev,path=/rootfs/vm1,security_model=none",
				"-device", "virtio-9p-pci,fsdev=rootdev,mount_tag=rootfs",
				"-nographic",
				"-no-reboot",
			},
		},
		{
			name: "node on two links",
			spec: driver.BootSpec{
				Name: "router", Kernel: "/cache/def456/Image", Rootfs: "/rootfs/router",
				Arch: "arm64", CPU: 1, MemMiB: 512, Taps: []string{"tap0", "tap1"},
			},
			want: []string{
				"qemu-system-aarch64",
				"-machine", "virt,gic-version=max",
				"-accel", "kvm",
				"-cpu", "host",
				"-smp", "1",
				"-m", "512",
				"-kernel", "/cache/def456/Image",
				"-append", "console=ttyAMA0 root=rootfs rootfstype=9p rootflags=trans=virtio,version=9p2000.L rw",
				"-fsdev", "local,id=rootdev,path=/rootfs/router,security_model=none",
				"-device", "virtio-9p-pci,fsdev=rootdev,mount_tag=rootfs",
				"-nographic",
				"-no-reboot",
				"-netdev", "tap,id=net0,ifname=tap0,script=no,downscript=no",
				"-device", "virtio-net-pci,netdev=net0",
				"-netdev", "tap,id=net1,ifname=tap1,script=no,downscript=no",
				"-device", "virtio-net-pci,netdev=net1",
			},
		},
	}

	d := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Argv(tt.spec)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("argv mismatch:\n got: %s\nwant: %s",
					strings.Join(got, " "), strings.Join(tt.want, " "))
			}
		})
	}
}

// TestArgvDefaults: an unset CPU/mem falls back to 1 vCPU / 512 MiB.
func TestArgvDefaults(t *testing.T) {
	got := New().Argv(driver.BootSpec{Kernel: "/k", Rootfs: "/r", Arch: "arm64"})
	joined := strings.Join(got, " ")
	for _, want := range []string{"-smp 1", "-m 512"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q missing default %q", joined, want)
		}
	}
}

// TestBootArgvLive pins the live boot command line: the extra `init=` cmdline
// and the second 9p control device are appended in a stable order, and the base
// flags are unchanged from the pure Argv.
func TestBootArgvLive(t *testing.T) {
	spec := driver.BootSpec{
		Name: "dev", Kernel: "/cache/abc123/Image", Rootfs: "/rootfs/dev",
		Arch: "arm64", CPU: 2, MemMiB: 1024,
	}
	got := bootArgv(spec, "init=/sbin/klab-init", "/run/single/dev/rw")
	want := []string{
		"qemu-system-aarch64",
		"-machine", "virt,gic-version=max",
		"-accel", "kvm",
		"-cpu", "host",
		"-smp", "2",
		"-m", "1024",
		"-kernel", "/cache/abc123/Image",
		"-append", "console=ttyAMA0 root=rootfs rootfstype=9p rootflags=trans=virtio,version=9p2000.L rw init=/sbin/klab-init",
		"-fsdev", "local,id=rootdev,path=/rootfs/dev,security_model=none",
		"-device", "virtio-9p-pci,fsdev=rootdev,mount_tag=rootfs",
		"-fsdev", "local,id=klabrw,path=/run/single/dev/rw,security_model=none",
		"-device", "virtio-9p-pci,fsdev=klabrw,mount_tag=klabrw",
		"-nographic",
		"-no-reboot",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("live argv mismatch:\n got: %s\nwant: %s",
			strings.Join(got, " "), strings.Join(want, " "))
	}
}
