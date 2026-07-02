package qemu

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zhengjieji/klab/internal/driver"
)

// The qemu driver reaches a booted node through a 9p file channel — no ssh, no
// network. The host writes a request file with the command; the in-guest init
// runs it and writes a response file. These helpers are the pure codec for that
// protocol and for the boot-readiness console line; live.go's Exec/Boot compose
// them, and they are table-tested here on hosted CI.

// encodeRequest serializes a command as the request-file body: each argv element
// is shell-quoted and space-joined into one command line, which the in-guest init
// runs with `sh -c`. Quoting keeps each element a single, literal word (so
// `klab exec … -- grep 'a b'` works) while still letting an explicit
// `sh -c '<script>'` use shell features.
func encodeRequest(argv []string) []byte {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = shQuote(a)
	}
	return []byte(strings.Join(q, " "))
}

// parseResponse parses a response-file body of the form "rc=<n>\n<output>" into
// an ExecResult. Stdout carries the merged command output (Stage 1 does not
// split stderr).
func parseResponse(b []byte) (driver.ExecResult, error) {
	head, body, found := strings.Cut(string(b), "\n")
	if !found || !strings.HasPrefix(head, "rc=") {
		return driver.ExecResult{}, fmt.Errorf("qemu: malformed exec response %q", head)
	}
	code, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(head, "rc=")))
	if err != nil {
		return driver.ExecResult{}, fmt.Errorf("qemu: bad exit code in exec response: %w", err)
	}
	return driver.ExecResult{ExitCode: code, Stdout: body}, nil
}

// parseReady scans a boot console log for the init's readiness line,
// "KLAB_READY <uname-r> <uname-m>", returning the kernel release and machine.
// ok is false until the line appears.
func parseReady(consoleLog string) (release, machine string, ok bool) {
	for _, line := range strings.Split(consoleLog, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && f[0] == "KLAB_READY" {
			return f[1], f[2], true
		}
	}
	return "", "", false
}
