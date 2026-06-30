# klab

[English](README.md) · **简体中文**

> 一个用于**自定义内核 Linux 拓扑**的声明式实验室——从单台 VM 到多节点集群——
> 在你的 Mac 上运行,并能原样移植到云端。

[![ci](https://github.com/zhengjieji/klab/actions/workflows/ci.yml/badge.svg)](https://github.com/zhengjieji/klab/actions/workflows/ci.yml)
[![license: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**状态:早期 / 开发中(pre-v0.1)。** 设计与路线图已确定(见 `docs/`),实现按阶段推进。

> 项目以英文为主进行开发:代码、注释、提交信息均用英文。文档提供中英双语,默认英文。

## 这是什么

内核、eBPF、网络方向的工作需要三样东西:能编译*自己的*内核、能搭*任意*拓扑
(单机、两台机器对连、N 节点集群),以及*可复现*的结果。今天要做到这些,意味着
手工拼 QEMU 命令行、网桥和 rootfs 镜像——而且在 Apple Silicon 上还开箱即跑不起来:
常见的 `qemu-system-x86_64 -accel kvm` 会悄悄退化成慢速软件模拟。

klab 把整个环境**声明式化**,并把 **Apple Silicon 作为一等开发目标**:

- **为 arm64 和 x86_64 编译内核**,在同一台主机上(LLVM 交叉编译)。
- **以节点形式启动它们**,通过可插拔驱动——目前 QEMU(完整 VM),下一步 Firecracker(microVM)。
- **用标准 Linux 网络搭任意拓扑**(网桥 / veth)。
- **运行实验并记录溯源信息**,让每个结果都钉死在确切的 内核 + 配置 + 拓扑 上。

## 工作原理

```
macOS (Apple M3/M4) · Virtualization.framework
└── Lima Linux VM (vz + 嵌套虚拟化 → /dev/kvm)              ← 一台硬件加速的 Linux 盒子
    ├── 编译: make LLVM=1 ARCH=arm64 | ARCH=x86_64
    ├── 网络: Linux 网桥 / veth  (任意图)
    └── 节点: driver = qemu | firecracker | cloud  (每个节点启动自己的内核)
```

Lima 这条线以上的一切都是普通 Linux——所以你在 Mac 上跑的同一份 `topology.yaml`,
也能在云端 Linux 机器上原样跑。Mac 只是托起了一台硬件加速的 Linux 盒子。

拓扑就是数据:

```yaml
name: dual
nodes:
  vm1: { driver: qemu, kernel: v6.17-bpf-arm64, arch: arm64, cpu: 2, mem: 1G, profile: bpf-min }
  vm2: { driver: qemu, kernel: v6.17-bpf-arm64, arch: arm64, cpu: 2, mem: 1G, profile: bpf-min }
links:
  - { name: data0, members: [vm1, vm2], subnet: 192.168.100.0/24 }
```

`single` 是一个节点;`dual` 是两个节点对连;集群就是 N 个节点挂在一条或多条链路上。
同一个引擎,不同的输入。

## 快速开始(目标体验——具体哪些已实现见路线图)

```sh
# 主机:Apple M3+ / macOS 15+
git clone https://github.com/zhengjieji/klab && cd klab
./scripts/setup.sh        # 检测环境、按需安装依赖、启动并验证加速 Linux 主机
klab doctor               # 随时复查主机就绪状态(芯片、macOS、/dev/kvm、内存/磁盘)

klab kernel build v6.17-bpf-arm64        # 编译自定义内核
klab up examples/topologies/single.yaml  # 以节点形式启动
klab ssh single dev                      # 进入 shell
klab down examples/topologies/single.yaml
```

详见 [`docs/quickstart.zh-CN.md`](docs/quickstart.zh-CN.md) 与
[`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)。

## 支持的平台

| 主机 | 能编译的内核 | 跑得快 | 能跑(慢) |
| --- | --- | --- | --- |
| Apple M3/M4, macOS 15+ | arm64 **和** x86_64 | arm64 (HVF/KVM) | x86_64 (TCG,仅功能验证) |
| Linux + KVM(云) | 与主机架构一致 | 主机架构(原生 KVM) | — |

> **关于 8GB Mac:** 适合 1–2 台自定义内核 VM;3 节点集群是很紧的、仅功能验证的挤压
> (用 microVM 驱动、让 macOS 保持精简)。真正的规模和可信的 x86 性能数据应放到云端
> 主机——同一份 spec 在那边原样运行。

## 与其它方案的对比
- **对比裸 QEMU 脚本:** 声明式拓扑、带缓存的内核矩阵、可复现的结果记录,而且在
  Apple Silicon 上真的有硬件加速。
- **对比 Lima/Colima/multipass:** 它们跑标准发行版 VM;klab 让每个节点启动*你自己的*
  内核,并把节点编织成*图*。
- **对比 kind/k3d:** 它们共享宿主内核(容器)。klab 给每个节点各自的内核——这是内核/
  eBPF 工作的硬性要求。

## 路线图
阶段 0 地基 · 1 编译+启动 · 2 拓扑引擎 · 3 实验+溯源 · 4 microVM 集群 ·
5 云/x86 + 发布。详见 [`docs/architecture.zh-CN.md`](docs/architecture.zh-CN.md)。

## 贡献
见 [`CONTRIBUTING.md`](CONTRIBUTING.md)。单元/lint 测试在任何机器上都能跑;live 的
KVM/启动测试需要 Apple Silicon(M3+)主机。

## 许可证
[MIT](LICENSE)。(也在考虑 Apache-2.0,因为它带明确的专利授权。)
