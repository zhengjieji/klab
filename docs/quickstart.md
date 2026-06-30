# Quickstart

> Status: the host setup (Stage 0) and kernel build + single-node boot (Stage 1) are
> the first milestones. Commands below marked _(roadmap)_ are not implemented yet.

## Prerequisites

- Apple **M3 or later** on **macOS 15+** (nested virtualization requires this).
- [lima](https://lima-vm.io): `brew install lima`
- [Go 1.22+](https://go.dev) to build the CLI: `brew install go`

> M1/M2 or Intel Macs can still build kernels and run nodes, but without nested KVM the
> microVM driver and in-VM acceleration are limited — see architecture.md.

## 1. Start the accelerated Linux host

```sh
limactl start scripts/lima/klab.yaml
```

This provisions a Lima VM with `vmType: vz` and `nestedVirtualization: true`, installs
the kernel build + QEMU toolchain, and verifies `/dev/kvm`.

## 2. Check readiness _(roadmap, Stage 0)_

```sh
klab doctor
```

Reports your chip, macOS version, lima status, `/dev/kvm`, and free RAM/disk, and tells
you what will and won't fit.

## 3. Build a kernel _(roadmap, Stage 1)_

```sh
klab kernel build v6.17-bpf-arm64      # native arm64
klab kernel build v6.17-bpf-x86_64     # cross-compiled x86 (build only on a Mac)
```

## 4. Bring up a topology _(roadmap, Stage 2)_

```sh
klab up examples/topologies/single.yaml
klab ssh single dev
klab status single
klab down examples/topologies/single.yaml
```

Today you can already validate a topology file:

```sh
klab validate examples/topologies/dual.yaml
```

## 8 GB Mac notes

Keep macOS lean while running VMs. Comfortable: 1–2 QEMU nodes. A 3-node k3s cluster is
a functional-only squeeze (microVM driver). For real scale or x86 performance, run the
same files on a cloud Linux host.
