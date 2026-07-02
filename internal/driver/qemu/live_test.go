package qemu

import (
	"context"
	"strings"
	"testing"

	"github.com/zhengjieji/klab/internal/driver"
	kexec "github.com/zhengjieji/klab/internal/exec"
)

// fakeRunner records every command and returns a canned result; a handler can
// override the result per call (matched on argv).
type fakeRunner struct {
	calls   [][]string
	handler func(argv []string) kexec.Result
}

func (f *fakeRunner) Run(_ context.Context, argv ...string) (kexec.Result, error) {
	f.calls = append(f.calls, argv)
	if f.handler != nil {
		return f.handler(argv), nil
	}
	return kexec.Result{ExitCode: 0}, nil
}

func (f *fakeRunner) sawContaining(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

func liveSpec() driver.BootSpec {
	return driver.BootSpec{
		Name: "dev", Kernel: "/cache/img/Image",
		Rootfs: "/run/dev/rootfs", RootfsRW: "/run/dev/rw",
		Arch: "arm64", CPU: 2, MemMiB: 1024,
	}
}

func TestLaunchArgv(t *testing.T) {
	got := launchArgv(liveSpec(), "/run/dev/console.log")
	joined := strings.Join(got, " ")
	// -nographic dropped, file serial + display added, init= appended, 2nd 9p present.
	for _, want := range []string{
		"-serial file:/run/dev/console.log", "-display none",
		"init=/sbin/klab-init", "mount_tag=klabrw",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("launchArgv missing %q in: %s", want, joined)
		}
	}
	if strings.Contains(joined, "-nographic") {
		t.Error("launchArgv should not contain -nographic")
	}
}

func TestBootReadyAndHandle(t *testing.T) {
	f := &fakeRunner{} // default: every command succeeds, so `test -e ready` passes immediately
	d := Driver{Runner: f}
	h, err := d.Boot(context.Background(), liveSpec())
	if err != nil {
		t.Fatal(err)
	}
	if string(h) != "/run/dev" {
		t.Errorf("handle = %q, want /run/dev", h)
	}
	if !f.sawContaining("setsid") || !f.sawContaining("init=/sbin/klab-init") {
		t.Error("Boot did not issue the detached qemu launch")
	}
}

func TestExecRoundTrip(t *testing.T) {
	f := &fakeRunner{handler: func(argv []string) kexec.Result {
		joined := strings.Join(argv, " ")
		if strings.Contains(joined, "cat ") && strings.Contains(joined, ".res") {
			return kexec.Result{ExitCode: 0, Stdout: "rc=0\nhello\n"}
		}
		return kexec.Result{ExitCode: 0}
	}}
	d := Driver{Runner: f}
	got, err := d.Exec(context.Background(), driver.Handle("/run/dev"), []string{"uname", "-r"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode != 0 || got.Stdout != "hello\n" {
		t.Errorf("Exec = %+v, want {0, \"hello\\n\"}", got)
	}
	if !f.sawContaining("base64 -d") {
		t.Error("Exec did not write a base64-encoded request")
	}
}

func TestStopKillsAndCleans(t *testing.T) {
	f := &fakeRunner{}
	d := Driver{Runner: f}
	if err := d.Stop(context.Background(), driver.Handle("/run/dev")); err != nil {
		t.Fatal(err)
	}
	if !f.sawContaining("qemu.pid") || !f.sawContaining("pkill") || !f.sawContaining("rm -rf") {
		t.Error("Stop should kill by pidfile, pkill the rootfs backstop, and remove the run dir")
	}
}
