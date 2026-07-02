package runner

import (
	"reflect"
	"testing"

	"github.com/zhengjieji/klab/internal/driver"
	"github.com/zhengjieji/klab/internal/topology"
)

func TestParseMem(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"1G", 1024, false},
		{"512M", 512, false},
		{"2g", 2048, false},
		{"256m", 256, false},
		{"2048", 2048, false},
		{"", 0, false},
		{"1T", 0, true},
		{"abc", 0, true},
		{"-1G", 0, true},
	}
	for _, tt := range tests {
		got, err := parseMem(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseMem(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("parseMem(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestResolveBootSpec(t *testing.T) {
	n := topology.Node{Driver: "qemu", Kernel: "bpf-arm64", Arch: "arm64", CPU: 2, Mem: "1G"}
	got, err := ResolveBootSpec("dev", n, "/cache/img/Image", "/run/rootfs", "/run/rw")
	if err != nil {
		t.Fatal(err)
	}
	want := driver.BootSpec{
		Name: "dev", Kernel: "/cache/img/Image", Rootfs: "/run/rootfs", RootfsRW: "/run/rw",
		Arch: "arm64", CPU: 2, MemMiB: 1024,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ResolveBootSpec = %+v, want %+v", got, want)
	}
	if _, err := ResolveBootSpec("dev", topology.Node{Mem: "bad"}, "", "", ""); err == nil {
		t.Error("ResolveBootSpec should propagate a bad memory size")
	}
}

func TestCheckCaps(t *testing.T) {
	qemuCaps := driver.Caps{CustomKernel: true, NeedsKVM: true, Arches: []string{"arm64"}}
	containerCaps := driver.Caps{CustomKernel: false, Arches: []string{"arm64"}}

	if err := CheckCaps("qemu", topology.Node{Kernel: "bpf-arm64", Arch: "arm64"}, qemuCaps); err != nil {
		t.Errorf("valid node rejected: %v", err)
	}
	if err := CheckCaps("container", topology.Node{Kernel: "bpf-arm64", Arch: "arm64"}, containerCaps); err == nil {
		t.Error("custom-kernel node on a no-kernel driver should be rejected")
	}
	if err := CheckCaps("qemu", topology.Node{Kernel: "k", Arch: "x86_64"}, qemuCaps); err == nil {
		t.Error("unsupported arch should be rejected")
	}
}
