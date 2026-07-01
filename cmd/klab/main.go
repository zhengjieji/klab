// Command klab is the CLI for the klab custom-kernel topology lab.
//
// This is the scaffold: `version`, `validate`, and `doctor` have working entry
// points; build/up/run/down are wired to clear "not yet implemented" stubs that
// map to the staged roadmap in docs/architecture.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zhengjieji/klab/internal/host"
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
	case "build", "kernel":
		notImplemented(cmd, "stage 1")
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
  klab kernel build <name>     build a kernel from the matrix                      [stage 1]
  klab up <topology>           bring up a topology                                 [stage 2]
  klab status <topology>       show node status                                    [stage 2]
  klab ssh <topology> <node>   shell into a node                                   [stage 2]
  klab down <topology>         tear down a topology                                [stage 2]
  klab run <experiment>        run an experiment (setup/run/collect)               [stage 3]
`)
}
