#!/bin/sh
# klab in-guest init (PID 1), delivered into the rootfs as /sbin/klab-init and
# booted via `init=/sbin/klab-init`.
#
# Lineage: Stanislav Fomichev's `q` script (auto-mount bpffs/cgroup2, 9p), minus
# the overlay pivot — klab's Stage-1 rootfs is a per-node rw 9p clone, and
# OVERLAY_FS is a module in this kernel. It:
#   - mounts proc/sys/dev + tmpfs on /run,/tmp
#   - mounts the shared control/scratch dir (second 9p device, tag klabrw) on /klab
#   - mounts bpffs + cgroup2 and brings up lo (F1.6 substrate)
#   - signals readiness on the console and via /klab/ready
#   - serves a simple exec command loop over /klab/ctl (the driver's Exec channel)
set -eu

mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true
mount -t tmpfs tmpfs /run
mount -t tmpfs tmpfs /tmp

# Shared control/scratch channel (second 9p device, tag klabrw).
mkdir -p /klab
mount -t 9p -o trans=virtio,version=9p2000.L,msize=512000 klabrw /klab

# F1.6 substrate: bpffs + cgroup2, and lo up for the XDP attach.
mount -t bpf bpf /sys/fs/bpf 2>/dev/null || true
mount -t cgroup2 cgroup2 /sys/fs/cgroup 2>/dev/null || true
ip link set lo up 2>/dev/null || true

mkdir -p /klab/ctl
echo "KLAB_READY $(uname -r) $(uname -m)" > /dev/console
: > /klab/ready

# Exec channel: for each <seq>.req (argv NUL-joined), run it and write back
# "rc=<n>\n<output>" as <seq>.res (atomically via a .tmp rename).
while :; do
	req=""
	for f in /klab/ctl/*.req; do
		[ -e "$f" ] && { req="$f"; break; }
	done
	if [ -z "$req" ]; then
		sleep 0.2
		continue
	fi
	seq=$(basename "$req" .req)
	cmd=$(cat "$req")
	out=$(sh -c "$cmd" 2>&1) && rc=0 || rc=$?
	{ printf 'rc=%s\n' "$rc"; printf '%s' "$out"; } > "/klab/ctl/$seq.res.tmp"
	mv "/klab/ctl/$seq.res.tmp" "/klab/ctl/$seq.res"
	rm -f "$req"
done
