// Package kernel models the kernel build matrix: named (ref, arch, config)
// tuples that build to cached, content-addressed artifacts.
//
// Builds happen inside the host Linux VM via `make LLVM=1 ARCH=<arch>`, which
// lets one toolchain cross-compile both arm64 and x86_64 from a single host.
package kernel

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
