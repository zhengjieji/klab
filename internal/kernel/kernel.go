// Package kernel models the kernel build matrix: named (ref, arch, config)
// tuples that build to cached, content-addressed artifacts.
//
// Builds happen inside the host Linux VM via `make LLVM=1 ARCH=<arch>`, which
// lets one toolchain cross-compile both arm64 and x86_64 from a single host.
// The config-merge and cache-key logic here is pure (no I/O), so it is the
// bulk of what hosted CI can test without building anything (see PLAN §7.2).
package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Spec is one entry in the kernel matrix (see kernels.yaml, Stage 3).
type Spec struct {
	Name       string   `yaml:"name"`
	Ref        string   `yaml:"ref"`        // git ref in the linux tree
	Arch       string   `yaml:"arch"`       // arm64 | x86_64
	BaseConfig string   `yaml:"baseConfig"` // path to a base .config
	Fragments  []string `yaml:"fragments"`  // config fragments layered on top
}

// Artifact is the result of a successful build.
type Artifact struct {
	Image   string // path to the bootable image (arm64 Image / x86 bzImage)
	Vmlinux string // path to vmlinux (for gdb / BTF)
	Config  string // resolved .config actually built
	Hash    string // content hash of the resolved inputs == cache key
}

var (
	reConfigSet = regexp.MustCompile(`^CONFIG_([A-Za-z0-9_]+)=(.*)$`)
	reConfigOff = regexp.MustCompile(`^# CONFIG_([A-Za-z0-9_]+) is not set$`)
)

// MergeConfig layers kconfig fragments over a base config with last-wins
// precedence: later fragments override earlier ones, and any fragment overrides
// the base. It understands both `CONFIG_X=<value>` and `# CONFIG_X is not set`,
// treats `CONFIG_X=n` and `# CONFIG_X is not set` as equivalent (disabled), and
// ignores blank lines and ordinary comments.
//
// The result is canonical: exactly one directive per symbol, sorted by symbol
// name, disabled symbols written in `# CONFIG_X is not set` form. Canonical
// output is what makes the merge deterministic and safe to feed into CacheKey.
func MergeConfig(base string, fragments ...string) string {
	vals := map[string]string{}
	apply := func(text string) {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if m := reConfigSet.FindStringSubmatch(line); m != nil {
				vals[m[1]] = m[2]
				continue
			}
			if m := reConfigOff.FindStringSubmatch(line); m != nil {
				vals[m[1]] = "n"
			}
		}
	}
	apply(base)
	for _, f := range fragments {
		apply(f)
	}

	syms := make([]string, 0, len(vals))
	for s := range vals {
		syms = append(syms, s)
	}
	sort.Strings(syms)

	var b strings.Builder
	for _, s := range syms {
		if vals[s] == "n" {
			fmt.Fprintf(&b, "# CONFIG_%s is not set\n", s)
		} else {
			fmt.Fprintf(&b, "CONFIG_%s=%s\n", s, vals[s])
		}
	}
	return b.String()
}

// CacheKey is the content-addressed build cache key: a SHA-256 over everything
// the build output depends on — the git ref, target arch, toolchain identity,
// and the fully-resolved .config. Identical inputs yield an identical key (a
// cache hit, F1.3); changing any fragment, the ref, arch, or toolchain changes
// it (a cache miss, F1.4). Because it hashes the *resolved* (merged, sorted)
// config, the order in which non-conflicting fragments were listed does not
// change the key.
//
// Fields are length-prefixed so that field boundaries cannot be forged by
// shifting characters between adjacent fields.
func CacheKey(ref, arch, toolchain, resolvedConfig string) string {
	h := sha256.New()
	for _, field := range []string{ref, arch, toolchain, resolvedConfig} {
		fmt.Fprintf(h, "%d:", len(field))
		io.WriteString(h, field)
	}
	return hex.EncodeToString(h.Sum(nil))
}
