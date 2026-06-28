# sbox-manager

`sbox-manager` 是基于 sing-box 的代理实例管理、订阅服务和流量统计项目。

目标：覆盖两个参考项目的功能点，不兼容旧配置、旧命令、旧订阅包或旧数据库格式；CLI 子命令集合在新语义下尽量贴近参考项目，降低使用习惯迁移成本。

本项目的功能点参考：

- `proxystack-go`：`/Users/eagle/Sync/proxy/proxystack-go`，实例配置、订阅服务、服务生命周期、安装诊断和备份能力。
- `xray traffic`：`/Users/eagle/.local/apps/init/tools/xray/traffic`，多实例流量采集、聚合、查询、导出和清理能力。

重要边界：本项目不考虑兼容旧配置、旧命令、旧订阅包或旧数据库格式，只复用功能需求和运维经验。

## 文档

- [总体架构方案](docs/architecture.md)
- [CLI 命令规格](docs/cli-spec.md)
- [配置与数据规格](docs/data-spec.md)
- [发布、安装与 Makefile 规格](docs/release.md)
- [开发规范](docs/conventions.md)
- [验收矩阵](docs/acceptance-matrix.md)
- [进度跟踪](docs/PROGRESS.md)
- [任务拆分](docs/tasks/)

## 当前状态

当前处于规格整理和实现准备状态，尚未进入 Go 源码实现。任务编号只表示落地顺序，不代表功能裁剪。
