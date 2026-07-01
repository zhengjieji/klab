// Command klab is the CLI for the klab custom-kernel topology lab.
//
// This is the scaffold: `version`, `validate`, and `doctor` have working entry
// points; build/up/run/down are wired to clear "not yet implemented" stubs that
// map to the staged roadmap in docs/architecture.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhengjieji/klab/internal/host"
	"github.com/zhengjieji/klab/internal/kernel"
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
	case "up", "down", "status", "ssh":
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

	var frags []string
	for _, f := range spec.Fragments {
		b, err := os.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "klab: reading config fragment %s: %v\n", f, err)
			os.Exit(1)
		}
		frags = append(frags, string(b))
	}
	merged := kernel.MergeConfig("", frags...)

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
  klab up <topology>           bring up a topology                                 [stage 2]
  klab status <topology>       show node status                                    [stage 2]
  klab ssh <topology> <node>   shell into a node                                   [stage 2]
  klab down <topology>         tear down a topology                                [stage 2]
  klab run <experiment>        run an experiment (setup/run/collect)               [stage 3]
`)
}
