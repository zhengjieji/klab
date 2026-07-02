package qemu

import (
	"reflect"
	"testing"

	"github.com/zhengjieji/klab/internal/driver"
)

func TestEncodeRequest(t *testing.T) {
	got := string(encodeRequest([]string{"bpftool", "prog", "list"}))
	want := "bpftool\x00prog\x00list"
	if got != want {
		t.Errorf("encodeRequest = %q, want %q", got, want)
	}
}

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    driver.ExecResult
		wantErr bool
	}{
		{"ok with output", "rc=0\nhello\n", driver.ExecResult{ExitCode: 0, Stdout: "hello\n"}, false},
		{"nonzero exit", "rc=1\nboom", driver.ExecResult{ExitCode: 1, Stdout: "boom"}, false},
		{"empty output", "rc=0\n", driver.ExecResult{ExitCode: 0, Stdout: ""}, false},
		{"multiline output", "rc=0\na\nb", driver.ExecResult{ExitCode: 0, Stdout: "a\nb"}, false},
		{"missing rc", "hello\n", driver.ExecResult{}, true},
		{"no newline", "rc=0", driver.ExecResult{}, true},
		{"bad code", "rc=x\n", driver.ExecResult{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResponse([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseReady(t *testing.T) {
	log := "[    0.5] booting\nrandom junk\nKLAB_READY 6.12.0 aarch64\nmore\n"
	r, m, ok := parseReady(log)
	if !ok || r != "6.12.0" || m != "aarch64" {
		t.Errorf("parseReady = (%q, %q, %v), want (6.12.0, aarch64, true)", r, m, ok)
	}
	if _, _, ok := parseReady("no marker here\n"); ok {
		t.Error("parseReady should report ok=false when the marker is absent")
	}
}
