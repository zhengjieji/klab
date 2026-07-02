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
	"net"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
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
		if n.Arch == "" {
			return fmt.Errorf("node %q: arch is required", name)
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
		if len(l.Members) == 0 {
			return fmt.Errorf("link %q: has no members", l.Name)
		}
		for _, m := range l.Members {
			if _, ok := t.Nodes[m]; !ok {
				return fmt.Errorf("link %q references unknown node %q", l.Name, m)
			}
		}
		if l.Subnet != "" {
			if _, _, err := net.ParseCIDR(l.Subnet); err != nil {
				return fmt.Errorf("link %q: invalid subnet %q (want CIDR like 192.168.100.0/24)", l.Name, l.Subnet)
			}
		}
	}
	return nil
}

// Instance is a concrete node after count-expansion. A node with count > 1
// becomes count instances named "<node>-<i>"; each gets a stable MAC.
type Instance struct {
	Name string
	Base string // the topology node key this instance came from (for link resolution)
	Node Node   // the node spec, with Count reset to 0
	MAC  string // unique, in the QEMU locally-administered OUI (52:54:00:…)
}

// Expand resolves each node's count into concrete instances, deterministically:
// nodes are taken in sorted name order, then by index, and MACs are assigned
// sequentially so the same topology always yields the same instances. A node
// with count <= 1 yields one instance under its own name.
func (t *Topology) Expand() []Instance {
	names := make([]string, 0, len(t.Nodes))
	for n := range t.Nodes {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Instance
	seq := 0
	add := func(instName, base string, n Node) {
		seq++
		n.Count = 0
		out = append(out, Instance{Name: instName, Base: base, Node: n, MAC: mac(seq)})
	}
	for _, name := range names {
		n := t.Nodes[name]
		count := n.Count
		if count < 1 {
			add(name, name, n)
			continue
		}
		for i := 0; i < count; i++ {
			add(fmt.Sprintf("%s-%d", name, i), name, n)
		}
	}
	return out
}

// mac returns a unique MAC in the QEMU OUI for the seq-th instance (1-based).
func mac(seq int) string {
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", (seq>>16)&0xff, (seq>>8)&0xff, seq&0xff)
}

// Load reads a topology YAML file, unmarshals it, and validates it.
func Load(path string) (*Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("topology %s: %w", path, err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// ValidateFile parses and validates a topology file.
func ValidateFile(path string) error {
	_, err := Load(path)
	return err
}
