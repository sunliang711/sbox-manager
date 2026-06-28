# 进度跟踪

## 当前状态

- 当前实施状态：T07 诊断、ipinfo、备份已完成并通过复审。
- 范围：覆盖两个参考项目的功能点，不兼容旧格式。
- 二进制命名：`sboxctl`、`sboxsub`。
- 功能参考：`/Users/eagle/Sync/proxy/proxystack-go` 和 `/Users/eagle/.local/apps/init/tools/xray/traffic`。

## 决策记录

| 决策 | 状态 |
| --- | --- |
| 使用 sing-box 单 core | 已确认 |
| 不考虑旧项目兼容性 | 已确认 |
| agent/sub 边界分离 | 已确认 |
| traffic 使用 Go 实现 | 已确认 |
| traffic 使用 baseline 自算 delta | 已确认 |
| 不兼容旧 mihomo load-balance 语义，用新模型表达对应能力 | 已确认 |
| 同时验收 Linux/systemd 和 macOS/launchd | 已确认 |
| SQLite 使用 GORM 管理模型和迁移 | 已确认 |
| 模板使用 Go 标准模板 | 已确认 |
| CLI 命令树贴近参考项目命名 | 已确认 |
| `start/restart` 先 apply runtime 再调用服务管理器，`restart` 显式强制重启 | 已确认 |
| 顶层备份使用 `backup_manifest.json`，不包含 runtime manifest 或 sboxsub inputs | 已确认 |
| traffic timer 通过 `sboxctl traffic timer` 管理 systemd/launchd 调度 | 已确认 |
| `sboxsub` 默认监听 loopback，公网部署需要显式配置监听和 token | 已确认 |
| runtime manifest 使用新 schema 记录受管 generated 文件和服务映射 | 已确认 |
| traffic 保留期按 hourly/daily/monthly 分层处理 | 已确认 |
| tag push 自动构建 GitHub Release 并上传校验过的二进制资产 | 待实现 |
| 安装脚本只安装二进制，不隐式初始化配置或安装服务 | 待实现 |

## 任务状态

| 任务 | 状态 |
| --- | --- |
| T01 项目骨架与 CLI | 已完成 |
| T02 配置模型与校验 | 已完成 |
| T03 sing-box 生成器与 Runtime | 已完成 |
| T04 实例生命周期与安装 | 已完成 |
| T05 订阅服务 | 已完成 |
| T06 流量统计 | 已完成 |
| T07 诊断、ipinfo、备份 | 已完成 |
| T08 测试与验收 | 未开始 |
| T09 Release、安装脚本与 Makefile | 未开始 |

## 开发入口

开发时按任务编号推进，每个任务完成后更新本文件。任务编号只表示落地顺序，不代表功能裁剪。
