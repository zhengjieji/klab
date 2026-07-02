// klab: a trivial XDP program that passes every packet.
//
// Used by the Stage 1 boot test (F1.6) to prove a BPF/XDP program loads and
// attaches to `lo` in-guest. Kept self-contained — it only needs linux/bpf.h
// (from linux-libc-dev), so it compiles with `clang -target bpf` on the build
// host without libbpf headers. The compiled object is shipped in the rootfs.
#include <linux/bpf.h>

#define SEC(name) __attribute__((section(name), used))

SEC("xdp")
int xdp_pass(struct xdp_md *ctx)
{
	(void)ctx;
	return XDP_PASS;
}

char _license[] SEC("license") = "GPL";
