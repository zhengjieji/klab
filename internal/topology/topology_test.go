package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	q := func() Node { return Node{Driver: "qemu"} }

	tests := []struct {
		name    string
		topo    Topology
		wantErr bool
	}{
		{"missing name", Topology{Nodes: map[string]Node{"a": q()}}, true},
		{"no nodes", Topology{Name: "x"}, true},
		{"node missing driver", Topology{Name: "x", Nodes: map[string]Node{"a": {}}}, true},
		{"negative count", Topology{Name: "x", Nodes: map[string]Node{"a": {Driver: "qemu", Count: -1}}}, true},
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
		"link to unknown node": write("ghost.yaml", "name: x\nnodes:\n  a:\n    driver: qemu\nlinks:\n  - name: l\n    members: [ghost]\n"),
	}
	for name, path := range cases {
		if _, err := Load(path); err == nil {
			t.Errorf("Load(%s): expected an error, got nil", name)
		}
	}
}
