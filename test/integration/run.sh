#!/usr/bin/env bash
# klab live integration tests (Stage 1: F1.5/F1.6/F1.7).
#
# Builds + boots the arm64 bpf-min kernel as a single node, asserts in-guest
# state, and tears it down cleanly. klab drives qemu INSIDE the Lima host, so
# /dev/kvm lives there (not on the Mac). Requires an M3+/macOS 15+ Lima host (or
# a KVM Linux box). NOT run by hosted CI. Run from the repo root: make test-live.
#
# Prerequisites (see docs/quickstart): the Lima host is up (scripts/setup.sh) and
# the bpf-min base rootfs is built (scripts/guest/mkrootfs.sh in the host VM).
set -euo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
BIN=./bin/klab
TOPO=examples/topologies/single.yaml
INSTANCE="${KLAB_LIMA_INSTANCE:-klab}"

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }
kx() { "$BIN" exec "$TOPO" dev -- "$@"; }
# Match by process name (comm), not full cmdline: pgrep -f would also match the
# wrapping shell whose arguments mention qemu. pgrep excludes its own process.
qemu_count() { limactl shell "$INSTANCE" -- pgrep -c qemu-system 2>/dev/null || true; }
tap_count() { limactl shell "$INSTANCE" -- sh -c 'ip -o link show type tun 2>/dev/null | wc -l' | tr -d ' '; }

command -v limactl >/dev/null || fail "limactl not found — run scripts/setup.sh"
limactl shell "$INSTANCE" -- test -e /dev/kvm || fail "/dev/kvm not in the '$INSTANCE' Lima host — run klab doctor"
[ -x "$BIN" ] || fail "$BIN missing — run: make build"

echo "== building kernel (cache hit if already built) =="
"$BIN" kernel build bpf-arm64 >/dev/null || fail "kernel build failed"

cleanup() { "$BIN" down "$TOPO" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== F1.5: boot + uname =="
"$BIN" up "$TOPO" >/dev/null || fail "klab up failed (is the bpf-min base rootfs built? see scripts/guest/mkrootfs.sh)"
unm="$(kx uname -rm)"
echo "  uname -rm: $unm"
case "$unm" in *aarch64*) ;; *) fail "F1.5: not aarch64: $unm" ;; esac
case "$unm" in 6.12.*) ;; *) fail "F1.5: not the built 6.12.x kernel: $unm" ;; esac
pass "F1.5 boots; uname -r=6.12.x, uname -m=aarch64"

echo "== F1.6: bpffs/cgroup2 + bpftool + XDP =="
[ "$(kx sh -c 'grep -cE "bpf|cgroup2" /proc/mounts')" -ge 2 ] || fail "F1.6: bpffs + cgroup2 not both mounted"
kx bpftool prog list >/dev/null || fail "F1.6: bpftool prog list failed"
kx sh -c 'ip link set dev lo xdpgeneric obj /usr/lib/klab/xdp_pass.o sec xdp' || fail "F1.6: XDP attach to lo failed"
[ "$(kx sh -c 'bpftool prog list | grep -ci xdp')" -ge 1 ] || fail "F1.6: attached XDP prog not listed by bpftool"
kx sh -c 'ip link set dev lo xdpgeneric off' >/dev/null 2>&1 || true
pass "F1.6 bpffs+cgroup2 mounted; bpftool works; XDP loads+attaches to lo"

echo "== F1.7: teardown leaves nothing =="
"$BIN" down "$TOPO" >/dev/null || fail "klab down failed"
trap - EXIT
sleep 1
[ "$(qemu_count)" -eq 0 ] || fail "F1.7: stray qemu process after down"
[ "$(tap_count)" -eq 0 ] || fail "F1.7: stray tap device after down"
pass "F1.7 clean teardown (no qemu, no taps)"

echo "[OK] Stage 1 live gate passed (F1.5 / F1.6 / F1.7)"
