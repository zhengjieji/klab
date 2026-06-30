// Package topology defines the declarative klab topology model: nodes, links,
// and the structural rules that bind them to drivers and kernels.
//
// A topology is plain data. `single` is one node with no links; `dual` is two
// nodes on one link; a cluster is N nodes across one or more links. The same
// runner materializes all of them.
package topology

import (
	"errors"
	"fmt"
	"os"
)

// Topology is the top-level declarative spec. See examples/topologies/*.yaml.
type Topology struct {
	Name  string          `yaml:"name"`
	Nodes map[string]Node `yaml:"nodes"`
	Links []Link          `yaml:"links"`
}

// Node is a single machine in the topology.
type Node struct {
	Driver  string `yaml:"driver"`  // qemu | firecracker | container | cloud
	Kernel  string `yaml:"kernel"`  // name of an entry in the kernel matrix
	Arch    string `yaml:"arch"`    // arm64 | x86_64
	CPU     int    `yaml:"cpu"`     // vCPUs
	Mem     string `yaml:"mem"`     // memory, e.g. "1G", "512M"
	Profile string `yaml:"profile"` // rootfs profile, e.g. bpf-min | k8s-node
	Count   int    `yaml:"count"`   // optional; >1 expands into a named group
}

// Link is an L2 segment (a Linux bridge) connecting a set of nodes.
type Link struct {
	Name    string   `yaml:"name"`
	Members []string `yaml:"members"`
	Subnet  string   `yaml:"subnet"` // CIDR
}

// Validate checks structural invariants that do not require a host or any I/O.
// Keeping this pure is deliberate: it is the bulk of what hosted CI can test.
func (t *Topology) Validate() error {
	if t.Name == "" {
		return errors.New("topology: name is required")
	}
	if len(t.Nodes) == 0 {
		return errors.New("topology: at least one node is required")
	}
	for name, n := range t.Nodes {
		if n.Driver == "" {
			return fmt.Errorf("node %q: driver is required", name)
		}
		if n.Count < 0 {
			return fmt.Errorf("node %q: count must be >= 0", name)
		}
	}
	seen := map[string]bool{}
	for _, l := range t.Links {
		if l.Name == "" {
			return errors.New("link: name is required")
		}
		if seen[l.Name] {
			return fmt.Errorf("link %q: duplicate name", l.Name)
		}
		seen[l.Name] = true
		for _, m := range l.Members {
			if _, ok := t.Nodes[m]; !ok {
				return fmt.Errorf("link %q references unknown node %q", l.Name, m)
			}
		}
	}
	return nil
}

// ValidateFile parses and validates a topology file.
//
// NOTE: YAML unmarshalling is a Stage-2 deliverable. For now this validates the
// file exists so the CLI and CI have a working end-to-end path; the structural
// Validate above is already fully unit-tested.
func ValidateFile(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	// TODO(stage-2): unmarshal YAML into Topology and call (*Topology).Validate().
	return nil
}
