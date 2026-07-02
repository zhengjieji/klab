// Command klab is the CLI for the klab custom-kernel topology lab.
//
// This is the scaffold: `version`, `validate`, and `doctor` have working entry
// points; build/up/run/down are wired to clear "not yet implemented" stubs that
// map to the staged roadmap in docs/architecture.md.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhengjieji/klab/internal/driver"
	"github.com/zhengjieji/klab/internal/driver/qemu"
	kexec "github.com/zhengjieji/klab/internal/exec"
	"github.com/zhengjieji/klab/internal/host"
	"github.com/zhengjieji/klab/internal/kernel"
	"github.com/zhengjieji/klab/internal/runner"
	"github.com/zhengjieji/klab/internal/topology"
)

const version = "0.0.0-dev"

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	switch cmd := os.Args[1]; cmd {
	case "version", "--version", "-v":
		fmt.Println("klab", version)
	case "help", "--help", "-h":
		usage(os.Stdout)
	case "validate":
		validateCmd(os.Args[2:])
	case "doctor":
		os.Exit(host.Run(os.Stdout))
	case "setup":
		runScript("setup.sh", os.Args[2:])
	case "kernel":
		kernelCmd(os.Args[2:])
	case "up":
		upCmd(os.Args[2:])
	case "down":
		downCmd(os.Args[2:])
	case "exec":
		execCmd(os.Args[2:])
	case "status":
		statusCmd(os.Args[2:])
	case "ssh":
		notImplemented(cmd, "stage 2")
	case "run":
		notImplemented(cmd, "stage 3")
	default:
		fmt.Fprintf(os.Stderr, "klab: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func validateCmd(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: klab validate <topology.yaml>")
		os.Exit(2)
	}
	if err := topology.ValidateFile(fs.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "invalid:", err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

// builtinSpecs is the Stage 1 kernel matrix. Stage 3 replaces this with a
// declarative kernels.yaml; for now the bpf-min entries are wired in code.
var builtinSpecs = map[string]kernel.Spec{
	"bpf-arm64":  {Name: "bpf-arm64", Ref: "v6.12", Arch: "arm64", BaseConfig: "defconfig", Fragments: []string{"configs/kernel/bpf-min.config"}},
	"bpf-x86_64": {Name: "bpf-x86_64", Ref: "v6.12", Arch: "x86_64", BaseConfig: "defconfig", Fragments: []string{"configs/kernel/bpf-min.config"}},
}

func specNames() []string {
	names := make([]string, 0, len(builtinSpecs))
	for n := range builtinSpecs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func kernelCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: klab kernel build <name> | klab kernel list")
		os.Exit(2)
	}
	switch args[0] {
	case "build":
		kernelBuild(args[1:])
	case "list":
		for _, n := range specNames() {
			s := builtinSpecs[n]
			fmt.Printf("%-12s linux %s  %s\n", n, s.Ref, s.Arch)
		}
	default:
		fmt.Fprintf(os.Stderr, "klab kernel: unknown subcommand %q\n", args[0])
		os.Exit(2)
	}
}

func kernelBuild(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: klab kernel build <name>")
		os.Exit(2)
	}
	spec, ok := builtinSpecs[args[0]]
	if !ok {
		fmt.Fprintf(os.Stderr, "klab: unknown kernel %q (have: %s)\n", args[0], strings.Join(specNames(), ", "))
		os.Exit(1)
	}

	merged, err := resolveConfig(spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "klab:", err)
		os.Exit(1)
	}

	store := &kernel.Store{
		Root:      cacheRoot(),
		Toolchain: toolchainID(),
		Builder:   kernel.MakeBuilder{},
	}
	fmt.Fprintf(os.Stderr, "klab: building %s (linux %s, %s, %s)\n", spec.Name, spec.Ref, spec.Arch, store.Toolchain)
	a, hit, err := store.Get(context.Background(), spec, merged)
	if err != nil {
		fmt.Fprintln(os.Stderr, "klab:", err)
		os.Exit(1)
	}
	status := "built"
	if hit {
		status = "cache hit (not rebuilt)"
	}
	fmt.Printf("%s: %s\n  image:  %s\n  config: %s\n  hash:   %s\n", spec.Name, status, a.Image, a.Config, a.Hash)
}

// cacheRoot is the content-addressed kernel cache on the host. It lives under
// ~/klab, which lima mounts into the VM at the same absolute path, so the in-VM
// builder writes images straight into it.
func cacheRoot() string {
	return filepath.Join(os.Getenv("HOME"), "klab", "cache")
}

// toolchainID identifies the in-VM LLVM toolchain for the cache key, coarsely
// (major version) so clang point releases do not churn the golden hash.
func toolchainID() string {
	out, err := exec.Command("limactl", "shell", "klab", "--", "clang", "--version").Output()
	if err == nil {
		if i := strings.Index(string(out), "clang version "); i >= 0 {
			major := strings.SplitN(strings.TrimSpace(string(out)[i+len("clang version "):]), ".", 2)[0]
			if major != "" {
				return "clang-" + major
			}
		}
	}
	return "clang"
}

// resolveConfig reads a spec's fragment files and merges them into the resolved
// config used both to build a kernel and to look one up in the cache.
func resolveConfig(spec kernel.Spec) (string, error) {
	var frags []string
	for _, f := range spec.Fragments {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("reading config fragment %s: %w", f, err)
		}
		frags = append(frags, string(b))
	}
	return kernel.MergeConfig("", frags...), nil
}

// resolveKernelImage returns the path of an already-built kernel image for a
// matrix name, or an error telling the user to build it first (it never builds).
func resolveKernelImage(name string) (string, error) {
	spec, ok := builtinSpecs[name]
	if !ok {
		return "", fmt.Errorf("unknown kernel %q (have: %s)", name, strings.Join(specNames(), ", "))
	}
	merged, err := resolveConfig(spec)
	if err != nil {
		return "", err
	}
	key := kernel.CacheKey(spec.Ref, spec.Arch, toolchainID(), merged)
	data, err := os.ReadFile(filepath.Join(cacheRoot(), key, "artifact.json"))
	if err != nil {
		return "", fmt.Errorf("kernel %q is not built; run 'klab kernel build %s' first", name, name)
	}
	var a kernel.Artifact
	if err := json.Unmarshal(data, &a); err != nil {
		return "", fmt.Errorf("corrupt cache metadata for %q: %w", name, err)
	}
	return a.Image, nil
}

// vmHome reports the run user's home inside the lima VM (paths are built from it).
func vmHome(ctx context.Context, r kexec.Runner) (string, error) {
	res, err := r.Run(ctx, "sh", "-c", "echo $HOME")
	if err != nil {
		return "", err
	}
	h := strings.TrimSpace(res.Stdout)
	if h == "" {
		return "", fmt.Errorf("could not determine the VM home directory")
	}
	return h, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "klab:", err)
	os.Exit(1)
}

// newCluster builds a Cluster bound to the lima host (any node count).
func newCluster(ctx context.Context) (*runner.Cluster, error) {
	r := kexec.LimaRunner{}
	home, err := vmHome(ctx, r)
	if err != nil {
		return nil, err
	}
	return &runner.Cluster{
		Runner: r,
		Driver: qemu.Driver{Runner: r},
		Home:   home,
		Base:   path.Join(home, ".cache/klab/rootfs/base/bpf-min"),
		Image:  resolveKernelImage,
	}, nil
}

func upCmd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: klab up <topology>")
		os.Exit(2)
	}
	topo, err := topology.Load(args[0])
	if err != nil {
		die(err)
	}
	ctx := context.Background()
	cl, err := newCluster(ctx)
	if err != nil {
		die(err)
	}
	if res, _ := cl.Runner.Run(ctx, "sudo", "test", "-e", cl.Base+"/sbin/klab-init"); res.ExitCode != 0 {
		die(fmt.Errorf("base rootfs missing at %s; run scripts/guest/mkrootfs.sh in the VM", cl.Base))
	}
	n := len(topo.Expand())
	fmt.Fprintf(os.Stderr, "klab: bringing up %s (%d node(s))...\n", topo.Name, n)
	if err := cl.Up(ctx, topo); err != nil {
		die(err)
	}
	for _, s := range cl.Status(ctx, topo) {
		fmt.Printf("  %s: %s\n", s.Name, stateOf(s))
	}
	fmt.Printf("%s: up (%d node(s))\n", topo.Name, n)
}

func downCmd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: klab down <topology>")
		os.Exit(2)
	}
	topo, err := topology.Load(args[0])
	if err != nil {
		die(err)
	}
	ctx := context.Background()
	cl, err := newCluster(ctx)
	if err != nil {
		die(err)
	}
	if err := cl.Down(ctx, topo); err != nil {
		die(err)
	}
	fmt.Printf("%s: down\n", topo.Name)
}

func statusCmd(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: klab status <topology>")
		os.Exit(2)
	}
	topo, err := topology.Load(args[0])
	if err != nil {
		die(err)
	}
	ctx := context.Background()
	cl, err := newCluster(ctx)
	if err != nil {
		die(err)
	}
	for _, s := range cl.Status(ctx, topo) {
		fmt.Printf("%-18s %s\n", s.Name, stateOf(s))
	}
}

func stateOf(s runner.NodeStatus) string {
	switch {
	case s.Reachable:
		return "ready"
	case s.Running:
		return "running"
	default:
		return "down"
	}
}

// execCmd runs a command inside a running node: klab exec <topology> <node> -- <cmd...>
func execCmd(args []string) {
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep < 2 || sep+1 >= len(args) {
		fmt.Fprintln(os.Stderr, "usage: klab exec <topology> <node> -- <cmd> [args...]")
		os.Exit(2)
	}
	topoPath, nodeName, cmd := args[0], args[1], args[sep+1:]
	topo, err := topology.Load(topoPath)
	if err != nil {
		die(err)
	}
	// Node names are the count-expanded instance names (e.g. "node-0"), not the
	// pre-expansion keys in topo.Nodes.
	known := false
	for _, in := range topo.Expand() {
		if in.Name == nodeName {
			known = true
			break
		}
	}
	if !known {
		die(fmt.Errorf("no node %q in topology %q", nodeName, topo.Name))
	}
	ctx := context.Background()
	r := kexec.LimaRunner{}
	home, err := vmHome(ctx, r)
	if err != nil {
		die(err)
	}
	run := path.Join(home, ".cache/klab/run", nodeName)
	d := qemu.Driver{Runner: r}
	res, err := d.Exec(ctx, driver.Handle(run), cmd)
	if err != nil {
		die(err)
	}
	fmt.Print(res.Stdout)
	os.Exit(res.ExitCode)
}

func notImplemented(cmd, stage string) {
	fmt.Fprintf(os.Stderr, "klab %s: not yet implemented (%s — see docs/architecture.md)\n", cmd, stage)
	os.Exit(1)
}

// runScript locates and runs scripts/<name>, forwarding args and streams. The
// auto-configuration logic (setup) lives in shell because it must run before
// Go/lima are even installed; the CLI is a thin front door. Read-only host
// detection (doctor) is Go instead — see internal/host — so its verdict logic
// is unit-testable against injected environments.
func runScript(name string, args []string) {
	candidates := []string{filepath.Join("scripts", name)}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "scripts", name),
			filepath.Join(filepath.Dir(exe), "..", "scripts", name),
		)
	}
	script := ""
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			script = c
			break
		}
	}
	if script == "" {
		fmt.Fprintf(os.Stderr, "klab: cannot find scripts/%s; run from the repo root or use `make`\n", name)
		os.Exit(1)
	}
	c := exec.Command("bash", append([]string{script}, args...)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "klab:", err)
		os.Exit(1)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `klab — a lab for custom-kernel Linux topologies

usage:
  klab version                 print version
  klab setup [--yes]           detect + auto-configure the host (installs deps, starts host)
  klab doctor                  check host readiness (chip, macOS, /dev/kvm, RAM)
  klab validate <file>         validate a topology file
  klab kernel list             list the kernels in the matrix
  klab kernel build <name>     build a kernel from the matrix (arm64/x86_64)
  klab up <topology>           boot a single-node topology
  klab exec <topo> <node> -- <cmd>   run a command inside a node
  klab down <topology>         tear a topology down
  klab status <topology>       show node status                                    [stage 2]
  klab ssh <topology> <node>   shell into a node                                   [stage 2]
  klab run <experiment>        run an experiment (setup/run/collect)               [stage 3]
`)
}
