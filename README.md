# klab

> A declarative lab for **custom-kernel Linux topologies** — from a single VM to a
> multi-node cluster — that runs on your Mac and ports unchanged to the cloud.

[![ci](https://github.com/zhengjieji/klab/actions/workflows/ci.yml/badge.svg)](https://github.com/zhengjieji/klab/actions/workflows/ci.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Status: early / work-in-progress (pre-v0.1).** The design and roadmap are settled
(see `docs/`), implementation is landing stage by stage.

## What it is

Kernel, eBPF, and networking work needs three things: build *your own* kernel, wire up
*arbitrary* topologies (one host, two hosts on a wire, an N-node cluster), and get
*reproducible* results. Today that means hand-rolling QEMU command lines, bridges, and
rootfs images — and it doesn't work out of the box on Apple silicon, where the usual
`qemu-system-x86_64 -accel kvm` quietly falls back to slow emulation.

klab makes the whole environment **declarative** and makes **Apple silicon a
first-class dev target**:

- **Build kernels for arm64 and x86_64** from one host (LLVM cross-compile).
- **Boot them as nodes** via pluggable drivers — QEMU (full VM) today, Firecracker
  (microVM) next.
- **Wire arbitrary topologies** with standard Linux networking (bridges / veth).
- **Run experiments** with captured provenance, so a result pins to an exact
  kernel + config + topology.

## How it works

```
macOS (Apple M3/M4) · Virtualization.framework
└── Lima Linux VM (vz + nested virtualization → /dev/kvm)   ← one accelerated Linux box
    ├── build:  make LLVM=1 ARCH=arm64 | ARCH=x86_64
    ├── fabric: Linux bridges / veth  (any graph)
    └── nodes:  driver = qemu | firecracker | cloud  (each boots its own kernel)
```

Everything above the Lima line is ordinary Linux — so the same `topology.yaml` you run
on the Mac runs on a cloud Linux box, no edits. The Mac just hosts one accelerated
Linux box.

A topology is data:

```yaml
name: dual
nodes:
  vm1: { driver: qemu, kernel: v6.17-bpf-arm64, arch: arm64, cpu: 2, mem: 1G, profile: bpf-min }
  vm2: { driver: qemu, kernel: v6.17-bpf-arm64, arch: arm64, cpu: 2, mem: 1G, profile: bpf-min }
links:
  - { name: data0, members: [vm1, vm2], subnet: 192.168.100.0/24 }
```

`single` is one node; `dual` is two on a wire; a cluster is N nodes on one or more
links. Same engine, different input.

## Quickstart (target UX — see roadmap for what's live)

```sh
# host: Apple M3+ / macOS 15+ with lima
limactl start scripts/lima/klab.yaml
klab doctor                              # verify chip, macOS, /dev/kvm, RAM/disk

klab kernel build v6.17-bpf-arm64        # build a custom kernel
klab up examples/topologies/single.yaml  # boot it as a node
klab ssh single dev                      # shell in
klab down examples/topologies/single.yaml
```

See [`docs/quickstart.md`](docs/quickstart.md) and
[`docs/architecture.md`](docs/architecture.md).

## Supported platforms

| Host | kernels you can build | what runs fast | what runs (slow) |
| --- | --- | --- | --- |
| Apple M3/M4, macOS 15+ | arm64 **and** x86_64 | arm64 (HVF/KVM) | x86_64 (TCG, functional only) |
| Linux + KVM (cloud) | matches host arch | host arch (native KVM) | — |

> **Note on 8 GB Macs:** great for 1–2 custom-kernel VMs; a 3-node cluster is a tight,
> functional-only squeeze (use the microVM driver, keep macOS lean). Real scale and
> faithful x86 performance belong on a cloud host — the same specs run there unchanged.

## How it compares
- **vs raw QEMU scripts:** declarative topologies, a kernel matrix with caching,
  reproducible result capture, and it actually accelerates on Apple silicon.
- **vs Lima/Colima/multipass:** those run standard-distro VMs; klab boots *your* kernel
  per node and builds *graphs* of nodes.
- **vs kind/k3d:** those share the host kernel (containers). klab gives each node its
  own kernel — required for kernel/eBPF work.

## Roadmap
Stage 0 substrate · 1 build+boot · 2 topology engine · 3 experiments+provenance ·
4 microVM cluster · 5 cloud/x86 + release. Details in
[`docs/architecture.md`](docs/architecture.md).

## Contributing
See [`CONTRIBUTING.md`](CONTRIBUTING.md). Unit/lint tests run anywhere; live KVM/boot
tests need an Apple-silicon (M3+) host.

## License
[MIT](LICENSE). (Apache-2.0 is under consideration for its explicit patent grant.)
