// Package exec runs commands inside the klab lima host VM. It is the single seam
// through which the impure "shell into lima" calls flow (qemu launch, rootfs
// clone, the 9p exec-channel file I/O, teardown), so the qemu driver's live
// Boot/Exec/Stop can be unit-tested against a fake Runner.
package exec

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Result is the outcome of a completed command. A non-zero ExitCode is reported
// in Result (not as an error); err is reserved for the command failing to run.
type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// Runner runs a command in the lima host VM and returns its captured output.
type Runner interface {
	Run(ctx context.Context, argv ...string) (Result, error)
}

// LimaRunner runs commands via `limactl shell <instance> -- <argv>`.
type LimaRunner struct {
	Instance string // default "klab"
}

func (r LimaRunner) instance() string {
	if r.Instance != "" {
		return r.Instance
	}
	return "klab"
}

// wrap prefixes argv with the limactl invocation. Pure, so it is unit-tested.
func wrap(instance string, argv []string) []string {
	return append([]string{"limactl", "shell", instance, "--"}, argv...)
}

// Run executes argv in the VM, waiting for completion and capturing output.
func (r LimaRunner) Run(ctx context.Context, argv ...string) (Result, error) {
	w := wrap(r.instance(), argv)
	cmd := exec.CommandContext(ctx, w[0], w[1:]...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil // the command ran; a non-zero exit is not a Runner error
		}
		return res, err
	}
	return res, nil
}
