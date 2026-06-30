#!/usr/bin/env bash
# klab in-guest init (placeholder — landed in Stage 1).
#
# Lineage: based on Stanislav Fomichev's `q` script. The real version runs as the
# guest init (`init=/bin/sh -- -c "...init.sh..."`) and:
#   - pivots onto an overlay over a 9p-mounted read-only rootfs
#   - mounts proc/sys/dev, bpffs, cgroup2; tunes bpf_jit sysctls
#   - mounts the kernel tree over 9p for modules/bpftool/perf
#   - configures the node's network interface(s) by MAC (one per link)
#   - starts sshd, then the requested workload (or an interactive shell)
#
# Until Stage 1 lands, this is a documented stub so the script path is stable.
set -euo pipefail
echo "klab guest init: not yet implemented (stage 1) — see docs/architecture.md" >&2
exit 1
