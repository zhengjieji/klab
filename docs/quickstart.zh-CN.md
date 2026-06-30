# 快速开始

[English](quickstart.md) · **简体中文**

> 状态:主机搭建(阶段 0)和 内核编译 + 单节点启动(阶段 1)是最先的里程碑。
> 下面标注 _(规划中)_ 的命令尚未实现。

## 前置条件

klab 不假设你已配好环境——`./scripts/setup.sh` 会做检测并按需自动配置。手动安装则需:

- Apple **M3 或更新**,**macOS 15+**(嵌套虚拟化需要此条件)。
- [lima](https://lima-vm.io):`brew install lima`
- [Go 1.22+](https://go.dev) 用于编译 CLI:`brew install go`

> M1/M2 或 Intel Mac 仍能编译内核、运行节点,但没有嵌套 KVM,microVM 驱动和 VM 内
> 加速会受限——见 architecture.zh-CN.md。

## 0. 一键检测 + 自动配置

```sh
./scripts/setup.sh          # 检测芯片/macOS/依赖,按需安装,启动并验证 Lima 主机
./scripts/setup.sh --yes    # 非交互(自动确认安装)
./scripts/doctor.sh         # 只检测、不改动,打印就绪报告
```

`setup.sh` 会:检查 Homebrew/lima/go(缺则提示或安装)、用
`scripts/lima/klab.yaml`(`vmType: vz` + `nestedVirtualization: true`)启动 Lima 主机、
并在 VM 内验证 `/dev/kvm`。脚本是幂等的,可重复运行。

## 1. 复查就绪状态 _(规划中,阶段 0:`klab doctor` 包装 `doctor.sh`)_

```sh
klab doctor
```

报告芯片、macOS 版本、lima 状态、`/dev/kvm`、空闲内存/磁盘,并告诉你什么能跑、什么跑不动。

## 2. 编译内核 _(规划中,阶段 1)_

```sh
klab kernel build v6.17-bpf-arm64      # 原生 arm64
klab kernel build v6.17-bpf-x86_64     # 交叉编译 x86(Mac 上只编不跑)
```

## 3. 启动拓扑 _(规划中,阶段 2)_

```sh
klab up examples/topologies/single.yaml
klab ssh single dev
klab status single
klab down examples/topologies/single.yaml
```

现在已经可以校验拓扑文件:

```sh
klab validate examples/topologies/dual.yaml
```

## 8GB Mac 注意事项

运行 VM 时让 macOS 保持精简。舒适区:1–2 台 QEMU 节点。3 节点 k3s 集群是仅功能验证的
挤压(microVM 驱动)。要真正的规模或 x86 性能,把同样的文件拿到云端 Linux 主机跑。
