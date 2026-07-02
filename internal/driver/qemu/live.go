package qemu

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/zhengjieji/klab/internal/driver"
	kexec "github.com/zhengjieji/klab/internal/exec"
)

// The live path. qemu runs inside the lima VM, so every step goes through the
// Runner (`limactl shell klab -- …`). A node's files live in a run directory
// (the Handle): <run>/rootfs is the per-node rw 9p root (cloned from the base by
// the caller), <run>/rw is the 9p control channel (tag klabrw), and <run>/
// {qemu.pid,console.log} are written by Boot. The rootfs clone + control dirs
// must already exist before Boot is called.

const (
	bootTimeout = 60 * time.Second
	execTimeout = 30 * time.Second
)

func (d Driver) runner() kexec.Runner {
	if d.Runner != nil {
		return d.Runner
	}
	return kexec.LimaRunner{}
}

// launchArgv is the detached-boot command line: bootArgv with `-nographic`
// swapped for a file-backed serial so the boot console is captured to a log.
func launchArgv(spec driver.BootSpec, console string) []string {
	a := bootArgv(spec, "init=/sbin/klab-init", spec.RootfsRW)
	out := make([]string, 0, len(a)+4)
	for _, x := range a {
		if x != "-nographic" {
			out = append(out, x)
		}
	}
	return append(out, "-display", "none", "-serial", "file:"+console)
}

// Boot launches the node's kernel under qemu (detached, in the VM) and waits for
// the in-guest init to signal readiness. Handle is the node's run directory.
func (d Driver) Boot(ctx context.Context, spec driver.BootSpec) (driver.Handle, error) {
	r := d.runner()
	run := path.Dir(spec.RootfsRW)
	console := path.Join(run, "console.log")
	pidfile := path.Join(run, "qemu.pid")
	ready := path.Join(spec.RootfsRW, "ready")

	// setsid + background so qemu outlives this call; record its pid.
	launch := fmt.Sprintf("%s </dev/null >/dev/null 2>&1 & echo $! > %s",
		shJoin(launchArgv(spec, console)), shQuote(pidfile))
	if _, err := r.Run(ctx, "sudo", "setsid", "bash", "-c", launch); err != nil {
		return "", fmt.Errorf("qemu: launch failed: %w", err)
	}

	deadline := time.Now().Add(bootTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	for time.Now().Before(deadline) {
		if res, err := r.Run(ctx, "sudo", "test", "-e", ready); err == nil && res.ExitCode == 0 {
			return driver.Handle(run), nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	tail, _ := r.Run(ctx, "sudo", "tail", "-n", "20", console)
	_, _ = r.Run(ctx, "sudo", "bash", "-c", "kill $(cat "+shQuote(pidfile)+" 2>/dev/null) 2>/dev/null || true")
	return "", fmt.Errorf("qemu: node %q did not become ready in %s; console tail:\n%s",
		spec.Name, bootTimeout, tail.Stdout)
}

// Exec runs argv inside the node over the 9p control channel: write a request
// file, wait for the response, parse "rc=<n>\n<output>".
func (d Driver) Exec(ctx context.Context, h driver.Handle, argv []string) (driver.ExecResult, error) {
	r := d.runner()
	ctl := path.Join(string(h), "rw", "ctl")
	seq := strconv.FormatInt(nowNano(), 10)
	req := path.Join(ctl, seq+".req")
	res := path.Join(ctl, seq+".res")

	// base64 carries the NUL-joined argv through the shell without quoting issues.
	b64 := base64.StdEncoding.EncodeToString(encodeRequest(argv))
	write := fmt.Sprintf("printf %%s %s | base64 -d > %s.tmp && mv %s.tmp %s",
		shQuote(b64), shQuote(req), shQuote(req), shQuote(req))
	if _, err := r.Run(ctx, "sudo", "bash", "-c", write); err != nil {
		return driver.ExecResult{}, fmt.Errorf("qemu: writing exec request: %w", err)
	}

	deadline := time.Now().Add(execTimeout)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	for time.Now().Before(deadline) {
		if got, err := r.Run(ctx, "sudo", "test", "-e", res); err == nil && got.ExitCode == 0 {
			out, err := r.Run(ctx, "sudo", "cat", res)
			if err != nil {
				return driver.ExecResult{}, err
			}
			_, _ = r.Run(ctx, "sudo", "rm", "-f", res)
			return parseResponse([]byte(out.Stdout))
		}
		time.Sleep(200 * time.Millisecond)
	}
	return driver.ExecResult{}, fmt.Errorf("qemu: exec %q timed out after %s", strings.Join(argv, " "), execTimeout)
}

// Stop kills the node's qemu (graceful, then forced), verifies it is gone, and
// removes the run directory. Idempotent: a missing pid or dead process is a
// no-op success, so no taps/mounts/processes leak (F1.7).
func (d Driver) Stop(ctx context.Context, h driver.Handle) error {
	r := d.runner()
	run := string(h)
	pidfile := path.Join(run, "qemu.pid")
	// Kill by recorded pid (graceful, then verify), then a backstop pkill on the
	// node's unique rootfs path to reap any stray qemu (e.g. a startup fork), so
	// no process leaks (F1.7). Finally remove the run dir.
	script := fmt.Sprintf(`pid=$(cat %s 2>/dev/null || true)
if [ -n "$pid" ]; then
  kill "$pid" 2>/dev/null || true
  for _ in $(seq 1 25); do kill -0 "$pid" 2>/dev/null || break; sleep 0.2; done
  kill -9 "$pid" 2>/dev/null || true
fi
pkill -9 -f %s 2>/dev/null || true
rm -rf %s`, shQuote(pidfile), shQuote(run+"/rootfs"), shQuote(run))
	if _, err := r.Run(ctx, "sudo", "bash", "-c", script); err != nil {
		return fmt.Errorf("qemu: stop failed: %w", err)
	}
	return nil
}

// nowNano is a small seam so Exec's sequence ids are deterministic in tests.
var nowNano = func() int64 { return time.Now().UnixNano() }

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func shJoin(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = shQuote(a)
	}
	return strings.Join(q, " ")
}
