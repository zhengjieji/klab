# Architecture

**English** · [简体中文](architecture.zh-CN.md)

klab runs custom-kernel Linux topologies on a Mac, and ports them unchanged to the
cloud. The trick is simple: **the Mac hosts one hardware-accelerated Linux box, and
everything else is ordinary Linux.**

## The layered model

```
macOS (Apple M3/M4) · Virtualization.framework (HVF)
└── Lima Linux VM (vz + nested virtualization → /dev/kvm)   ← one accelerated Linux box
    ├── kernel build      one LLVM toolchain → arm64 Image + x86_64 bzImage
    ├── rootfs store      shared read-only base images + per-node CoW overlays
    ├── topology fabric   Linux bridges / veth / netns   (any graph)
    └── nodes             driver = qemu | firecracker | (container) | (cloud)
                          each node boots its OWN kernel
```

Because everything above the Lima line is standard Linux, the same `topology.yaml`
runs on a cloud Linux box with native KVM. The Mac is just a convenient, accelerated
Linux host.

## Four orthogonal axes

1. **Kernel matrix** — named `(ref, arch, baseConfig, fragments)` built to
   content-addressed artifacts. Cache key is a hash of the resolved inputs, so an
   unchanged kernel is never rebuilt, and a changed fragment always is.
2. **Topology** — declarative `nodes` + `links`. `single`/`dual`/cluster are just
   different inputs to one runner.
3. **Driver** — the backend that boots one node. A node declares what it needs
   (custom kernel? which arch?); a driver declares what it can do (`Caps`). The runner
   matches them and fails fast on impossible combinations.
4. **Experiment** — `setup`/`run`/`collect` hooks plus a provenance manifest, so a
   result pins to an exact kernel + config + topology.

You can change any one axis without touching the others.

## Drivers and the 8 GB / cloud split

| Driver | Custom kernel | Footprint | Use |
| --- | --- | --- | --- |
| `qemu` | yes (arm64 fast, x86 TCG) | ~1–2 GB/node | kernel dev, kgdb, x86 functional — the workhorse |
| `firecracker` | yes (same-arch KVM only) | ~128–512 MB/node | node density; tiny clusters |
| `container` | **no** (shared kernel) | tiny | only for no-custom-kernel topologies |
| `cloud` | yes | remote | scale + faithful x86 performance |

On Apple silicon, only **same-arch** guests get hardware acceleration. So arm64 is the
fast local path; x86 guests run under QEMU TCG (functional, slow) — for faithful x86
performance, run the same specs on a cloud x86 host.

## Networking

Each `link` is a Linux bridge; nodes attach via taps. Multi-subnet graphs use
`veth`/routing. This is standard Linux networking, so arbitrary topologies — two hosts
on a wire, a routed multi-subnet lab, a k8s cluster network — need no new primitives,
and behave the same on the Mac and in the cloud.

## Non-goals

- Not a Docker/OCI replacement — klab boots kernels, not containers.
- Not a production cluster manager — it is a research lab.
- Not a benchmarking oracle on Apple silicon — x86 perf belongs on x86 silicon.

## Roadmap

| Stage | Delivers |
| --- | --- |
| 0 | substrate: Lima vz + nested virt; `klab doctor` |
| 1 | kernel build (arm64 + x86) + single-node arm64 boot |
| 2 | topology engine (N nodes, M links); reproduce dual-vm |
| 3 | experiment lifecycle + provenance + kernel matrix |
| 4 | microVM (Firecracker) driver + tiny custom-kernel k3s cluster |
| 5 | cloud/x86 engine + release |
