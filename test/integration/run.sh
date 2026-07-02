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
bridge_count() { limactl shell "$INSTANCE" -- sh -c 'ip -o link show type bridge 2>/dev/null | grep -c klbr-' | tr -d ' '; }

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

# ---- Stage 2: multi-node topologies ----

DUAL=examples/topologies/dual.yaml
echo "== F2.1/F2.5: dual node ping + status =="
trap '"$BIN" down "$DUAL" >/dev/null 2>&1' EXIT
"$BIN" up "$DUAL" >/dev/null || fail "klab up dual failed"
"$BIN" status "$DUAL" | grep -q "vm1.*ready" || fail "F2.5: status does not report vm1 ready"
"$BIN" exec "$DUAL" vm1 -- busybox ping -c 2 -W 2 192.168.100.2 | grep -q "0% packet loss" ||
	fail "F2.1: vm1 cannot ping vm2 over the link subnet"
pass "F2.1 dual vm1->vm2 ping; F2.5 status ready"

echo "== F2.2: XDP smoke (vm2's traffic hits an XDP prog on vm1) =="
"$BIN" exec "$DUAL" vm1 -- sh -c 'ip link set dev eth0 xdpgeneric obj /usr/lib/klab/xdp_drop.o sec xdp' ||
	fail "F2.2: XDP attach on vm1 eth0 failed"
"$BIN" exec "$DUAL" vm1 -- sh -c 'ip link show eth0 | grep -q xdpgeneric' ||
	fail "F2.2: XDP prog not attached on vm1 eth0"
# With XDP_DROP attached, vm2's traffic must not get through cleanly (expect loss).
if "$BIN" exec "$DUAL" vm2 -- busybox ping -c 3 -W 2 192.168.100.1 | grep -q "0% packet loss"; then
	fail "F2.2: XDP_DROP on vm1 did not affect vm2's traffic (0% loss with prog attached)"
fi
"$BIN" exec "$DUAL" vm1 -- sh -c 'ip link set dev eth0 xdpgeneric off' || fail "F2.2: XDP detach failed"
"$BIN" exec "$DUAL" vm2 -- busybox ping -c 3 -W 2 192.168.100.1 | grep -q "0% packet loss" ||
	fail "F2.2: connectivity not restored after detach"
pass "F2.2 XDP on vm1 eth0 sees vm2 traffic (attach->loss; detach->restored)"

"$BIN" down "$DUAL" >/dev/null || fail "klab down dual failed"
trap - EXIT

TRIO=examples/topologies/trio.yaml
echo "== F2.3/R2.4: 3-node all-pairs ping + clean teardown =="
trap '"$BIN" down "$TRIO" >/dev/null 2>&1' EXIT
"$BIN" up "$TRIO" >/dev/null || fail "klab up trio failed"
for src in node-0 node-1 node-2; do
	for i in 1 2 3; do
		"$BIN" exec "$TRIO" "$src" -- busybox ping -c 1 -W 2 "192.168.100.$i" | grep -q "0% packet loss" ||
			fail "F2.3: $src cannot reach 192.168.100.$i"
	done
done
"$BIN" down "$TRIO" >/dev/null || fail "klab down trio failed"
trap - EXIT
sleep 1
[ "$(qemu_count)" -eq 0 ] || fail "R2.4: stray qemu after down"
[ "$(tap_count)" -eq 0 ] || fail "R2.4: stray tap after down"
[ "$(bridge_count)" -eq 0 ] || fail "R2.4: stray bridge after down"
pass "F2.3 3-node all-pairs ping; R2.4 clean N-node teardown (no qemu/taps/bridges)"

echo "[OK] Stage 1+2 live gate passed (F1.5-F1.7, F2.1/F2.3/F2.5, R2.4)"
