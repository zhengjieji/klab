# 架构

[English](architecture.md) · **简体中文**

klab 在 Mac 上运行自定义内核的 Linux 拓扑,并能原样移植到云端。诀窍很简单:
**Mac 托起一台硬件加速的 Linux 盒子,其余一切都是普通 Linux。**

## 分层模型

```
macOS (Apple M3/M4) · Virtualization.framework (HVF)
└── Lima Linux VM (vz + 嵌套虚拟化 → /dev/kvm)             ← 一台硬件加速的 Linux 盒子
    ├── 内核编译      一套 LLVM 工具链 → arm64 Image + x86_64 bzImage
    ├── rootfs 仓库   共享只读基底镜像 + 每节点 CoW 叠加层
    ├── 拓扑网络      Linux 网桥 / veth / netns   (任意图)
    └── 节点          driver = qemu | firecracker | (container) | (cloud)
                      每个节点启动自己的内核
```

因为 Lima 线以上的一切都是标准 Linux,同一份 `topology.yaml` 能在带原生 KVM 的云端
Linux 机器上运行。Mac 只是一台方便、加速的 Linux 主机。

## 四条正交的轴

1. **内核矩阵** —— 命名的 `(ref, arch, baseConfig, fragments)`,编译成按内容寻址的产物。
   缓存键是对解析后输入的哈希,所以未改动的内核不会重编,改动的 fragment 一定重编。
2. **拓扑** —— 声明式的 `nodes` + `links`。`single`/`dual`/集群只是同一个 runner 的不同输入。
3. **驱动(driver)** —— 启动单个节点的后端。节点声明它*需要什么*(自定义内核?哪种架构?);
   驱动声明它*能做什么*(`Caps`)。runner 做匹配,对不可能的组合提前失败。
4. **实验** —— `setup`/`run`/`collect` 钩子,加上一份溯源 manifest,让结果钉死在确切的
   内核 + 配置 + 拓扑上。

四条轴互不影响:改其中一条不必动其它。

## 驱动与 8GB / 云端的分工

| 驱动 | 自定义内核 | 占用 | 用途 |
| --- | --- | --- | --- |
| `qemu` | 是(arm64 快,x86 走 TCG) | ~1–2 GB/节点 | 内核开发、kgdb、x86 功能验证——主力 |
| `firecracker` | 是(仅同架构 KVM) | ~128–512 MB/节点 | 节点密度;小集群 |
| `container` | **否**(共享内核) | 极小 | 仅用于不需要自定义内核的拓扑 |
| `cloud` | 是 | 远程 | 规模 + 可信 x86 性能 |

在 Apple Silicon 上,只有**同架构**客机能获得硬件加速。所以 arm64 是本地快速路径;
x86 客机走 QEMU TCG(能用、慢)——要可信的 x86 性能,把同一份 spec 拿到云端 x86 主机跑。

## 网络

每条 `link` 是一个 Linux 网桥;节点通过 tap 接入。多子网图用 `veth`/路由。这都是
标准 Linux 网络,所以任意拓扑——两台机器对连、带路由的多子网实验、k8s 集群网络——
不需要新原语,而且在 Mac 和云端表现一致。

## 非目标

- 不是 Docker/OCI 的替代品——klab 启动的是内核,不是容器。
- 不是生产级集群管理器——它是个研究实验室。
- 不是 Apple Silicon 上的性能基准权威——x86 性能数据属于 x86 硬件。

## 路线图

| 阶段 | 交付 |
| --- | --- |
| 0 | 地基:Lima vz + 嵌套虚拟化;`klab doctor` |
| 1 | 内核编译(arm64 + x86)+ 单节点 arm64 启动 |
| 2 | 拓扑引擎(N 节点,M 链路);复刻 dual-vm |
| 3 | 实验生命周期 + 溯源 + 内核矩阵 |
| 4 | microVM(Firecracker)驱动 + 小型自定义内核 k3s 集群 |
| 5 | 云/x86 引擎 + 发布 |
