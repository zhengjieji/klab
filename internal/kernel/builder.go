package kernel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// MakeBuilder builds kernels inside a Lima VM with `make LLVM=1 ARCH=<arch>`,
// cross-compiling arm64 and x86_64 from one LLVM toolchain. It shells into the
// instance via limactl; because the cache outDir sits on the virtiofs mount
// shared with the guest at the *same* absolute path, built images land exactly
// where the host Store expects them, with no path translation.
//
// This is the impure edge of the package — exercised end-to-end by
// `klab kernel build`, not by unit tests (the Store's cache logic is unit-tested
// against a fake Builder instead).
type MakeBuilder struct {
	Instance string // lima instance; default "klab"
	SrcRoot  string // guest dir for linux source trees (native fs); default ~/.cache/klab/src
	Jobs     int    // make -j; default 3 (capped for the 8 GB-class RAM budget)
}

func (b MakeBuilder) instance() string {
	if b.Instance != "" {
		return b.Instance
	}
	return "klab"
}

func (b MakeBuilder) srcRoot() string {
	if b.SrcRoot != "" {
		return b.SrcRoot
	}
	return "$HOME/.cache/klab/src"
}

func (b MakeBuilder) jobs() int {
	if b.Jobs > 0 {
		return b.Jobs
	}
	return 3
}

// archParams maps a klab arch to the kernel's ARCH=, its image make target, and
// the built image's path within the tree.
func archParams(arch string) (karch, target, imageRel string, err error) {
	switch arch {
	case "arm64":
		return "arm64", "Image", "arch/arm64/boot/Image", nil
	case "x86_64":
		return "x86_64", "bzImage", "arch/x86/boot/bzImage", nil
	default:
		return "", "", "", fmt.Errorf("kernel: unsupported arch %q", arch)
	}
}

// Build fetches linux at spec.Ref, configures it from spec.BaseConfig plus the
// resolved fragment, cross-compiles the image, and copies the image, vmlinux,
// and final .config into outDir (which is host- and guest-visible via virtiofs).
func (b MakeBuilder) Build(ctx context.Context, spec Spec, resolvedConfig, outDir string) (Artifact, error) {
	karch, target, imageRel, err := archParams(spec.Arch)
	if err != nil {
		return Artifact{}, err
	}
	ver := strings.TrimPrefix(spec.Ref, "v")              // v6.12 -> 6.12
	series := "v" + strings.SplitN(ver, ".", 2)[0] + ".x" // 6.12 -> v6.x
	url := fmt.Sprintf("https://cdn.kernel.org/pub/linux/kernel/%s/linux-%s.tar.xz", series, ver)

	// The fragment is applied over the base config target (e.g. defconfig); the
	// final .config is copied back out so it is the artifact's resolved config.
	script := fmt.Sprintf(`set -euo pipefail
SRC=%[1]s
mkdir -p "$SRC" && cd "$SRC"
[ -f linux-%[2]s.tar.xz ] || curl -fsSL -O %[3]s
[ -d linux-%[2]s ] || tar xf linux-%[2]s.tar.xz
cd linux-%[2]s
make LLVM=1 ARCH=%[4]s %[5]s
cat > klab.fragment <<'KLAB_FRAGMENT_EOF'
%[6]s
KLAB_FRAGMENT_EOF
./scripts/kconfig/merge_config.sh -m .config klab.fragment
make LLVM=1 ARCH=%[4]s olddefconfig
make LLVM=1 ARCH=%[4]s -j%[7]d %[8]s
cp %[9]s %[10]s/%[8]s
cp vmlinux %[10]s/vmlinux
cp .config %[10]s/config
`, b.srcRoot(), ver, url, karch, spec.BaseConfig, resolvedConfig, b.jobs(), target, imageRel, outDir)

	cmd := exec.CommandContext(ctx, "limactl", "shell", b.instance(), "--", "bash", "-c", script)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return Artifact{}, fmt.Errorf("kernel: build of %s (%s) failed: %w", spec.Name, spec.Arch, err)
	}
	return Artifact{
		Image:   filepath.Join(outDir, target),
		Vmlinux: filepath.Join(outDir, "vmlinux"),
		Config:  filepath.Join(outDir, "config"),
	}, nil
}
