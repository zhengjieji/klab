package runner

import (
	"fmt"
	"net"

	"github.com/zhengjieji/klab/internal/topology"
)

// NetPlan is the pure, snapshot-testable realization of a topology's networking:
// one Linux bridge per link, one tap per node-interface, and IPs assigned within
// each link's subnet. The live runner materializes exactly this; keeping it pure
// data is what lets R2.1 golden-test single/dual/N-node without a host.
type NetPlan struct {
	Bridges []Bridge
	Taps    []Tap
}

// Bridge is one L2 segment (a Linux bridge) realizing a link.
type Bridge struct {
	Name   string // bridge device name (<= 15 chars)
	Link   string // source link name
	Subnet string // CIDR, or "" for an L2-only link
}

// Tap is one node's interface on a bridge.
type Tap struct {
	Name   string // tap device name (<= 15 chars)
	Bridge string // bridge it attaches to
	Node   string // instance name
	MAC    string // interface MAC (QEMU OUI)
	IP     string // CIDR, e.g. 192.168.100.1/24 (empty when the link has no subnet)
}

// Plan computes the network plan for a topology: it expands node counts, then
// for each link creates a bridge and, for every member instance (in link-member
// order), a tap with a sequential name/MAC and an IP taken from the subnet by
// position (.1, .2, …). Deterministic, so the result is stable to snapshot.
func Plan(t *topology.Topology) (*NetPlan, error) {
	byBase := map[string][]string{}
	for _, in := range t.Expand() {
		byBase[in.Base] = append(byBase[in.Base], in.Name)
	}

	np := &NetPlan{}
	seq := 0
	for _, l := range t.Links {
		br := bridgeName(l.Name)
		np.Bridges = append(np.Bridges, Bridge{Name: br, Link: l.Name, Subnet: l.Subnet})

		var members []string
		for _, m := range l.Members {
			members = append(members, byBase[m]...)
		}

		var ipnet *net.IPNet
		var base net.IP
		if l.Subnet != "" {
			ip, n, err := net.ParseCIDR(l.Subnet)
			if err != nil {
				return nil, fmt.Errorf("link %q: invalid subnet %q", l.Name, l.Subnet)
			}
			ipnet, base = n, ip.Mask(n.Mask)
		}

		for i, name := range members {
			seq++
			tp := Tap{Name: fmt.Sprintf("klabtap%d", seq), Bridge: br, Node: name, MAC: macFor(seq)}
			if ipnet != nil {
				ones, _ := ipnet.Mask.Size()
				tp.IP = fmt.Sprintf("%s/%d", addToIPv4(base, i+1), ones)
			}
			np.Taps = append(np.Taps, tp)
		}
	}
	return np, nil
}

// bridgeName derives a Linux bridge device name from a link name, capped at the
// 15-char interface-name limit.
func bridgeName(link string) string {
	n := "klbr-" + link
	if len(n) > 15 {
		n = n[:15]
	}
	return n
}

// macFor returns a unique MAC in the QEMU OUI for the seq-th interface (1-based).
func macFor(seq int) string {
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", (seq>>16)&0xff, (seq>>8)&0xff, seq&0xff)
}

// addToIPv4 returns base + n as an IPv4 string (base is a network address).
func addToIPv4(base net.IP, n int) string {
	v4 := base.To4()
	if v4 == nil {
		return base.String()
	}
	val := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	val += uint32(n)
	return net.IPv4(byte(val>>24), byte(val>>16), byte(val>>8), byte(val)).String()
}
