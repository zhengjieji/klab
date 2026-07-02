package runner

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zhengjieji/klab/internal/topology"
)

func loadExample(t *testing.T, name string) *topology.Topology {
	t.Helper()
	topo, err := topology.Load(filepath.Join("..", "..", "examples", "topologies", name))
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return topo
}

// TestPlanGolden snapshots the bridge/tap/IP plan for single/dual/3-node so a
// change to the materialization (or an example) can't silently regress (R2.1).
func TestPlanGolden(t *testing.T) {
	tests := []struct {
		file string
		want NetPlan
	}{
		{
			file: "single.yaml",
			want: NetPlan{}, // no links -> no bridges, no taps
		},
		{
			file: "dual.yaml",
			want: NetPlan{
				Bridges: []Bridge{{Name: "klbr-data0", Link: "data0", Subnet: "192.168.100.0/24"}},
				Taps: []Tap{
					{Name: "klabtap1", Bridge: "klbr-data0", Node: "vm1", MAC: "52:54:00:00:00:01", IP: "192.168.100.1/24"},
					{Name: "klabtap2", Bridge: "klbr-data0", Node: "vm2", MAC: "52:54:00:00:00:02", IP: "192.168.100.2/24"},
				},
			},
		},
		{
			file: "k3s-cluster.yaml", // 3 instances (server + agent x2) on one link
			want: NetPlan{
				Bridges: []Bridge{{Name: "klbr-clusternet", Link: "clusternet", Subnet: "10.42.0.0/24"}},
				Taps: []Tap{
					{Name: "klabtap1", Bridge: "klbr-clusternet", Node: "server", MAC: "52:54:00:00:00:01", IP: "10.42.0.1/24"},
					{Name: "klabtap2", Bridge: "klbr-clusternet", Node: "agent-0", MAC: "52:54:00:00:00:02", IP: "10.42.0.2/24"},
					{Name: "klabtap3", Bridge: "klbr-clusternet", Node: "agent-1", MAC: "52:54:00:00:00:03", IP: "10.42.0.3/24"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := Plan(loadExample(t, tt.file))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("plan mismatch for %s:\n got:  %+v\n want: %+v", tt.file, *got, tt.want)
			}
		})
	}
}
