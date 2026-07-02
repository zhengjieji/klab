package runner

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/zhengjieji/klab/internal/driver"
	kexec "github.com/zhengjieji/klab/internal/exec"
	"github.com/zhengjieji/klab/internal/topology"
)

// Cluster boots and tears down a whole topology on the lima host: it realizes
// the NetPlan (one bridge per link, a tap per node-interface), boots every
// instance via the driver, and assigns per-NIC IPs by MAC. Single/dual/N-node
// are all the same path — a single node is just a topology with no links.
type Cluster struct {
	Runner kexec.Runner
	Driver driver.Driver
	Home   string                              // VM home; run dirs live under <Home>/.cache/klab/run
	Base   string                              // base rootfs path in the VM
	Image  func(kernel string) (string, error) // resolve a kernel-matrix name to its built image
}

func (c *Cluster) runDir(name string) string { return path.Join(c.Home, ".cache/klab/run", name) }

// Up creates the bridges + taps, boots every instance, and assigns per-NIC IPs.
func (c *Cluster) Up(ctx context.Context, topo *topology.Topology) error {
	plan, err := Plan(topo)
	if err != nil {
		return err
	}

	for _, b := range plan.Bridges {
		if err := c.createBridge(ctx, b.Name); err != nil {
			return fmt.Errorf("bridge %q: %w", b.Name, err)
		}
	}

	nics := map[string][]driver.NIC{}
	for _, t := range plan.Taps {
		if err := c.createTap(ctx, t.Name, t.Bridge); err != nil {
			return fmt.Errorf("tap %q: %w", t.Name, err)
		}
		nics[t.Node] = append(nics[t.Node], driver.NIC{Tap: t.Name, MAC: t.MAC})
	}

	for _, in := range topo.Expand() {
		img, err := c.Image(in.Node.Kernel)
		if err != nil {
			return fmt.Errorf("node %q: %w", in.Name, err)
		}
		if err := CheckCaps(in.Node.Driver, in.Node, c.Driver.Capabilities()); err != nil {
			return err
		}
		run := c.runDir(in.Name)
		if err := c.prepRootfs(ctx, run); err != nil {
			return fmt.Errorf("node %q rootfs: %w", in.Name, err)
		}
		spec, err := ResolveBootSpec(in.Name, in.Node, img, run+"/rootfs", run+"/rw")
		if err != nil {
			return err
		}
		spec.Nics = nics[in.Name]
		if _, err := c.Driver.Boot(ctx, spec); err != nil {
			return fmt.Errorf("node %q: %w", in.Name, err)
		}
	}

	// Configure IPs by MAC so a multi-NIC node gets the right address per link.
	for _, t := range plan.Taps {
		if t.IP == "" {
			continue
		}
		res, err := c.Driver.Exec(ctx, driver.Handle(c.runDir(t.Node)), ipConfigCmd(t.MAC, t.IP))
		if err != nil {
			return fmt.Errorf("node %q: assigning %s: %w", t.Node, t.IP, err)
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("node %q: no interface with MAC %s to assign %s", t.Node, t.MAC, t.IP)
		}
	}
	return nil
}

// Down stops every node and deletes the taps and bridges. Best-effort and
// idempotent, so nothing leaks (R2.4).
func (c *Cluster) Down(ctx context.Context, topo *topology.Topology) error {
	plan, _ := Plan(topo)
	for _, in := range topo.Expand() {
		_ = c.Driver.Stop(ctx, driver.Handle(c.runDir(in.Name)))
	}
	var links []string
	for _, t := range plan.Taps {
		links = append(links, t.Name)
	}
	for _, b := range plan.Bridges {
		links = append(links, b.Name)
	}
	if len(links) > 0 {
		script := fmt.Sprintf("for l in %s; do ip link del \"$l\" 2>/dev/null || true; done", strings.Join(links, " "))
		_, _ = c.Runner.Run(ctx, "sudo", "bash", "-c", script)
	}
	return nil
}

// NodeStatus is a node's live state.
type NodeStatus struct {
	Name      string
	Running   bool // qemu process alive
	Reachable bool // responds on the exec channel
}

// Status reports each instance's running + reachable state (F2.5).
func (c *Cluster) Status(ctx context.Context, topo *topology.Topology) []NodeStatus {
	var out []NodeStatus
	for _, in := range topo.Expand() {
		run := c.runDir(in.Name)
		st := NodeStatus{Name: in.Name}
		if r, err := c.Runner.Run(ctx, "sudo", "bash", "-c",
			"kill -0 \"$(cat "+run+"/qemu.pid 2>/dev/null)\" 2>/dev/null"); err == nil && r.ExitCode == 0 {
			st.Running = true
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			if res, err := c.Driver.Exec(pingCtx, driver.Handle(run), []string{"true"}); err == nil && res.ExitCode == 0 {
				st.Reachable = true
			}
			cancel()
		}
		out = append(out, st)
	}
	return out
}

func (c *Cluster) createBridge(ctx context.Context, name string) error {
	s := fmt.Sprintf(`ip link del %[1]s 2>/dev/null || true
ip link add %[1]s type bridge
ip link set %[1]s up
ip link set %[1]s type bridge stp_state 0 2>/dev/null || true
modprobe br_netfilter 2>/dev/null || true
echo 0 > /proc/sys/net/bridge/bridge-nf-call-iptables 2>/dev/null || true
iptables -P FORWARD ACCEPT 2>/dev/null || true`, name)
	_, err := c.Runner.Run(ctx, "sudo", "bash", "-c", s)
	return err
}

func (c *Cluster) createTap(ctx context.Context, tap, bridge string) error {
	s := fmt.Sprintf(`ip tuntap add %[1]s mode tap 2>/dev/null || true
ip link set %[1]s master %[2]s
ip link set %[1]s up`, tap, bridge)
	_, err := c.Runner.Run(ctx, "sudo", "bash", "-c", s)
	return err
}

func (c *Cluster) prepRootfs(ctx context.Context, run string) error {
	s := fmt.Sprintf("rm -rf %[1]s && mkdir -p %[1]s/rw/ctl && cp -a %[2]s %[1]s/rootfs", run, c.Base)
	_, err := c.Runner.Run(ctx, "sudo", "bash", "-c", s)
	return err
}

// ipConfigCmd finds the interface with the given MAC and assigns it the IP.
func ipConfigCmd(mac, ip string) []string {
	s := fmt.Sprintf(`for f in /sys/class/net/*/address; do
  if [ "$(cat "$f")" = "%s" ]; then
    d=$(basename "$(dirname "$f")")
    ip addr add %s dev "$d" && ip link set "$d" up && exit 0
  fi
done
exit 1`, mac, ip)
	return []string{"sh", "-c", s}
}
