// klab: a trivial XDP program that drops every packet.
//
// Used by the Stage 2 smoke test (F2.2) to prove that traffic from another node
// reaches an XDP program attached on a node's link interface: with this attached
// on vm1's eth0, pings from vm2 are dropped; detaching restores connectivity.
// Self-contained — only needs linux/bpf.h, so it compiles with `clang -target bpf`.
#include <linux/bpf.h>

#define SEC(name) __attribute__((section(name), used))

SEC("xdp")
int xdp_drop(struct xdp_md *ctx)
{
	(void)ctx;
	return XDP_DROP;
}

char _license[] SEC("license") = "GPL";
