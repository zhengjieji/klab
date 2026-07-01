package host

import (
	"strings"
	"testing"
)

// readyMac is a fully-provisioned M3+ host: every check passes, zero warnings.
// Individual cases below deviate one axis at a time so the verdict shift is
// unambiguous.
func readyMac() Facts {
	return Facts{
		OS: "darwin", ChipBrand: "Apple M3", AppleGen: 3, MacOSMajor: 15,
		MemGiB: 16, FreeDiskGiB: 100,
		HasBrew: true, HasLimactl: true, HasGo: true,
		LimaStatus: "Running", KVMPresent: true, KVMOK: true,
	}
}

func with(base Facts, mut func(*Facts)) Facts { mut(&base); return base }

// find returns the first check whose message contains sub.
func find(r Report, sub string) (Check, bool) {
	for _, c := range r.Checks {
		if strings.Contains(c.Message, sub) {
			return c, true
		}
	}
	return Check{}, false
}

func TestEvaluate(t *testing.T) {
	type want struct {
		level Level
		note  bool // require a non-empty actionable note
	}
	tests := []struct {
		name      string
		facts     Facts
		wantReady bool
		wantFails int
		wantWarns int
		contains  map[string]want // substring -> expected check
	}{
		{
			name:      "ready M3 16G",
			facts:     readyMac(),
			wantReady: true, wantFails: 0, wantWarns: 0,
			contains: map[string]want{
				"nested virtualization supported": {Pass, false},
				"/dev/kvm present":                {Pass, false},
			},
		},
		{
			name:      "M1 chip warns (no nested virt)",
			facts:     with(readyMac(), func(f *Facts) { f.ChipBrand = "Apple M1"; f.AppleGen = 1 }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"no nested virtualization": {Warn, false}},
		},
		{
			// F0.4: /dev/kvm missing -> not ready, exit non-zero, actionable message.
			name:      "M3 kvm missing (F0.4)",
			facts:     with(readyMac(), func(f *Facts) { f.KVMPresent = false; f.KVMOK = false }),
			wantReady: false, wantFails: 1, wantWarns: 0,
			contains: map[string]want{"/dev/kvm missing": {Fail, true}},
		},
		{
			name:      "kvm present but kvm-ok unconfirmed warns",
			facts:     with(readyMac(), func(f *Facts) { f.KVMOK = false }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"kvm-ok did not confirm": {Warn, false}},
		},
		{
			name:      "RAM 8G warns",
			facts:     with(readyMac(), func(f *Facts) { f.MemGiB = 8 }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"RAM 8 GiB": {Warn, false}},
		},
		{
			name:      "RAM 4G fails",
			facts:     with(readyMac(), func(f *Facts) { f.MemGiB = 4 }),
			wantReady: false, wantFails: 1, wantWarns: 0,
			contains: map[string]want{"< 8 GiB is very tight": {Fail, true}},
		},
		{
			name:      "macOS 14 fails",
			facts:     with(readyMac(), func(f *Facts) { f.MacOSMajor = 14 }),
			wantReady: false, wantFails: 1, wantWarns: 0,
			contains: map[string]want{"need 15+": {Fail, true}},
		},
		{
			name:      "low free disk warns",
			facts:     with(readyMac(), func(f *Facts) { f.FreeDiskGiB = 10 }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"want ~60 GiB": {Warn, false}},
		},
		{
			name:      "unknown free disk warns",
			facts:     with(readyMac(), func(f *Facts) { f.FreeDiskGiB = -1 }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"could not read free disk": {Warn, false}},
		},
		{
			name: "no tooling fails thrice",
			facts: with(readyMac(), func(f *Facts) {
				f.HasBrew, f.HasLimactl, f.HasGo = false, false, false
				f.LimaStatus = ""
			}),
			// brew + limactl + go fail; lima checks skipped (no limactl).
			wantReady: false, wantFails: 3, wantWarns: 0,
			contains: map[string]want{
				"Homebrew not found": {Fail, true},
				"limactl not found":  {Fail, true},
				"go not found":       {Fail, true},
			},
		},
		{
			name:      "lima not created warns",
			facts:     with(readyMac(), func(f *Facts) { f.LimaStatus = ""; f.KVMPresent = false; f.KVMOK = false }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"lima instance not created": {Warn, false}},
		},
		{
			name:      "lima stopped warns",
			facts:     with(readyMac(), func(f *Facts) { f.LimaStatus = "Stopped"; f.KVMPresent = false; f.KVMOK = false }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"exists but is": {Warn, false}},
		},
		{
			name:      "Intel mac chip warns",
			facts:     with(readyMac(), func(f *Facts) { f.ChipBrand = "Intel(R) Core(TM)"; f.AppleGen = 0; f.Intel = true }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"Intel Mac": {Warn, false}},
		},
		{
			name:      "unclassified chip warns",
			facts:     with(readyMac(), func(f *Facts) { f.ChipBrand = ""; f.AppleGen = 0; f.Intel = false }),
			wantReady: true, wantFails: 0, wantWarns: 1,
			contains: map[string]want{"could not classify": {Warn, false}},
		},
		{
			name: "linux ready",
			facts: Facts{
				OS: "linux", KVMPresent: true, KVMOK: true,
				HasGo: true, HasQEMU: true, HasClang: true, FreeDiskGiB: 100,
			},
			wantReady: true, wantFails: 0, wantWarns: 0,
		},
		{
			name: "linux no kvm fails",
			facts: Facts{
				OS: "linux", KVMPresent: false,
				HasGo: true, HasQEMU: true, HasClang: true,
			},
			wantReady: false, wantFails: 1, wantWarns: 0,
			contains: map[string]want{"/dev/kvm missing": {Fail, true}},
		},
		{
			name: "linux missing tools fail each",
			facts: Facts{
				OS: "linux", KVMPresent: true, KVMOK: true,
			},
			wantReady: false, wantFails: 3, wantWarns: 0,
			contains: map[string]want{
				"go not found":                  {Fail, true},
				"qemu-system-aarch64 not found": {Fail, true},
				"clang not found":               {Fail, true},
			},
		},
		{
			name:      "unsupported OS fails",
			facts:     Facts{OS: "windows"},
			wantReady: false, wantFails: 1, wantWarns: 0,
			contains: map[string]want{"unsupported OS": {Fail, true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Evaluate(tt.facts)
			if r.Ready() != tt.wantReady {
				t.Errorf("Ready() = %v, want %v", r.Ready(), tt.wantReady)
			}
			wantExit := 0
			if !tt.wantReady {
				wantExit = 1
			}
			if r.ExitCode() != wantExit {
				t.Errorf("ExitCode() = %d, want %d", r.ExitCode(), wantExit)
			}
			if r.Fails != tt.wantFails {
				t.Errorf("Fails = %d, want %d (checks: %+v)", r.Fails, tt.wantFails, r.Checks)
			}
			if r.Warns != tt.wantWarns {
				t.Errorf("Warns = %d, want %d (checks: %+v)", r.Warns, tt.wantWarns, r.Checks)
			}
			for sub, w := range tt.contains {
				c, ok := find(r, sub)
				if !ok {
					t.Errorf("no check containing %q; checks: %+v", sub, r.Checks)
					continue
				}
				if c.Level != w.level {
					t.Errorf("check %q level = %v, want %v", sub, c.Level, w.level)
				}
				if w.note && c.Note == "" {
					t.Errorf("check %q: want a non-empty actionable note", sub)
				}
			}
		})
	}
}
