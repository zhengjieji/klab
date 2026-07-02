#!/usr/bin/env bash
# klab: build the bpf-min base rootfs inside the host VM.
#
# Produces a minimal arm64 Ubuntu rootfs with bpftool + iproute2 (for F1.6), the
# klab init at /sbin/klab-init, and a precompiled XDP object at
# /usr/lib/klab/xdp_pass.o. `klab up` clones this base per node (cp -a) and boots
# the clone read-write over 9p; the base is built once and reused.
#
# Runs inside the lima VM on native fs (not the 9p mount) so device nodes and
# chroot work. Idempotent.
set -euo pipefail

BASE="${KLAB_ROOTFS_BASE:-$HOME/.cache/klab/rootfs/base/bpf-min}"
SUITE="${KLAB_ROOTFS_SUITE:-noble}"
MIRROR="${KLAB_ROOTFS_MIRROR:-http://ports.ubuntu.com/ubuntu-ports}"
SRC="${KLAB_SRC:-$HOME/.cache/klab/src}"
KVER="${KLAB_KERNEL_VER:-6.12}"
GUEST_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MULTIARCH="$(gcc -print-multiarch 2>/dev/null || echo aarch64-linux-gnu)"

log() { printf '[mkrootfs] %s\n' "$*"; }
die() {
	printf '[mkrootfs][error] %s\n' "$*" >&2
	exit 1
}

if [ -e "$BASE/sbin/klab-init" ]; then
	log "base already present: $BASE"
	exit 0
fi

# bpftool: Ubuntu ships it only as a virtual, kernel-version-coupled package, so
# build a matching one from the kernel source instead. Disable the LLVM/libbfd
# disassembler (feature-llvm=0) so its only runtime deps are libc/libelf/libz/
# libzstd -- all already in the minbase + iproute2 closure. Built native (the VM
# is arm64), so any linux-<ver>-* tree works.
tree="$(find "$SRC" -maxdepth 1 -type d -name "linux-$KVER-*" 2>/dev/null | head -1 || true)"
[ -n "$tree" ] || die "no linux-$KVER source tree under $SRC (run 'klab kernel build' first)"
log "building bpftool from $tree (no LLVM/libbfd disasm)"
make -C "$tree/tools/bpf/bpftool" feature-llvm=0 feature-libbfd=0 -j"$(nproc)" >/dev/null

log "debootstrap minbase (arm64, $SUITE) -> $BASE"
sudo debootstrap --arch=arm64 --variant=minbase \
	--include=iproute2,busybox-static "$SUITE" "$BASE" "$MIRROR"

log "installing bpftool, klab init, and the precompiled XDP program"
sudo install -D -m0755 "$tree/tools/bpf/bpftool/bpftool" "$BASE/usr/sbin/bpftool"
sudo install -D -m0755 "$GUEST_DIR/init.sh" "$BASE/sbin/klab-init"
clang -O2 -g -target bpf -I"/usr/include/$MULTIARCH" -c "$GUEST_DIR/xdp_pass.c" -o /tmp/klab_xdp_pass.o
sudo install -D -m0644 /tmp/klab_xdp_pass.o "$BASE/usr/lib/klab/xdp_pass.o"
rm -f /tmp/klab_xdp_pass.o

log "done: $BASE"
