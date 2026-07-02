package topology

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidate(t *testing.T) {
	q := func() Node { return Node{Driver: "qemu", Arch: "arm64"} }

	tests := []struct {
		name    string
		topo    Topology
		wantErr bool
	}{
		{"missing name", Topology{Nodes: map[string]Node{"a": q()}}, true},
		{"no nodes", Topology{Name: "x"}, true},
		{"node missing driver", Topology{Name: "x", Nodes: map[string]Node{"a": {Arch: "arm64"}}}, true},
		{"node missing arch", Topology{Name: "x", Nodes: map[string]Node{"a": {Driver: "qemu"}}}, true},
		{"negative count", Topology{Name: "x", Nodes: map[string]Node{"a": {Driver: "qemu", Arch: "arm64", Count: -1}}}, true},
		{"ok single", Topology{Name: "x", Nodes: map[string]Node{"a": q()}}, false},
		{
			"link to unknown node",
			Topology{Name: "x", Nodes: map[string]Node{"a": q()}, Links: []Link{{Name: "l", Members: []string{"ghost"}}}},
			true,
		},
		{
			"duplicate link name",
			Topology{Name: "x", Nodes: map[string]Node{"a": q()}, Links: []Link{{Name: "l", Members: []string{"a"}}, {Name: "l", Members: []string{"a"}}}},
			true,
		},
		{
			"link with no members",
			Topology{Name: "x", Nodes: map[string]Node{"a": q()}, Links: []Link{{Name: "l"}}},
			true,
		},
		{
			"bad CIDR",
			Topology{Name: "x", Nodes: map[string]Node{"a": q()}, Links: []Link{{Name: "l", Members: []string{"a"}, Subnet: "not-a-cidr"}}},
			true,
		},
		{
			"ok dual",
			Topology{Name: "x", Nodes: map[string]Node{"a": q(), "b": q()}, Links: []Link{{Name: "data0", Members: []string{"a", "b"}, Subnet: "192.168.100.0/24"}}},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.topo.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestLoadExamples: every shipped example topology parses and validates.
func TestLoadExamples(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join("..", "..", "examples", "topologies", "*.yaml"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no example topologies found: %v", err)
	}
	for _, m := range matches {
		if _, err := Load(m); err != nil {
			t.Errorf("Load(%s) failed: %v", filepath.Base(m), err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cases := map[string]string{
		"missing file":         filepath.Join(dir, "does-not-exist.yaml"),
		"malformed yaml":       write("bad.yaml", "name: x\nnodes: [oops\n"),
		"missing name":         write("noname.yaml", "nodes:\n  a:\n    driver: qemu\n"),
		"link to unknown node": write("ghost.yaml", "name: x\nnodes:\n  a:\n    driver: qemu\n    arch: arm64\nlinks:\n  - name: l\n    members: [ghost]\n"),
	}
	for name, path := range cases {
		if _, err := Load(path); err == nil {
			t.Errorf("Load(%s): expected an error, got nil", name)
		}
	}
}

func TestExpand(t *testing.T) {
	topo := Topology{
		Name: "c",
		Nodes: map[string]Node{
			"server": {Driver: "firecracker", Arch: "arm64"},
			"agent":  {Driver: "firecracker", Arch: "arm64", Count: 2},
		},
	}
	got := topo.Expand()
	// Sorted names: agent (count 2) -> agent-0, agent-1; then server.
	want := []string{"agent-0", "agent-1", "server"}
	if len(got) != len(want) {
		t.Fatalf("Expand() returned %d instances, want %d: %+v", len(got), len(want), got)
	}
	macs := map[string]bool{}
	for i, inst := range got {
		if inst.Name != want[i] {
			t.Errorf("instance %d name = %q, want %q", i, inst.Name, want[i])
		}
		if inst.Node.Count != 0 {
			t.Errorf("instance %q Count = %d, want 0", inst.Name, inst.Node.Count)
		}
		if macs[inst.MAC] {
			t.Errorf("duplicate MAC %q", inst.MAC)
		}
		macs[inst.MAC] = true
	}
	if got[0].MAC != "52:54:00:00:00:01" {
		t.Errorf("first MAC = %q, want 52:54:00:00:00:01", got[0].MAC)
	}
	if !reflect.DeepEqual(got, topo.Expand()) {
		t.Error("Expand() is not deterministic")
	}
}

// TestLoadRejectsDuplicateNodeKeys covers F2.6's "duplicate node name": the YAML
// parser rejects a duplicate mapping key.
func TestLoadRejectsDuplicateNodeKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dup.yaml")
	body := "name: x\nnodes:\n  a:\n    driver: qemu\n    arch: arm64\n  a:\n    driver: qemu\n    arch: arm64\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Error("Load should reject a duplicate node key")
	}
}
