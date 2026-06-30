#!/usr/bin/env bash
# klab live integration tests.
#
# These exercise the irreducibly-hardware path: building a kernel, booting it,
# asserting in-guest state, and tearing down cleanly. They require /dev/kvm
# (Apple M3+/macOS 15+ via the Lima host, or any Linux host with KVM) and are
# NOT run by hosted CI. See ../../CONTRIBUTING.md and ../../../PLAN.md §7.
set -euo pipefail

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }

[ -e /dev/kvm ] || fail "/dev/kvm not present — run inside the klab Lima host (M3+/macOS 15+) or a KVM Linux box"

# Stage gates are implemented incrementally; each becomes a real test as it lands.
# Stage 1: build v6.17-bpf-arm64; klab up single; assert uname -m == aarch64,
#          bpftool prog list works; klab down leaves no stray qemu/taps/mounts.
# Stage 2: klab up dual; vm1 pings vm2; teardown clean for N nodes.
# Stage 3: klab run <demo>; manifest provenance reproducible.
echo "[skip] no live stages implemented yet — scaffold only"
pass "integration harness present; /dev/kvm available"
