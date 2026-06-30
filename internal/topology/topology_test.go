package topology

import "testing"

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
