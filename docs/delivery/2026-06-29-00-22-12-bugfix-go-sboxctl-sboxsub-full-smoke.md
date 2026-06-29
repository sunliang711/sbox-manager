# sboxctl / sboxsub 全量远程 Smoke 修复记录

## 问题背景

- 测试环境：`root@10.2.100.157`，Debian 13 amd64，systemd。
- 构建方式：本地交叉编译 `linux/amd64` 后上传 `/usr/local/bin/sboxctl` 和 `/usr/local/bin/sboxsub`。
- 测试范围：`sboxctl`、`sboxsub` 根命令和全部子命令 help，初始化配置注释，实例模板注释，订阅导入导出，HTTP 订阅服务，systemd 生命周期，资源安装更新卸载，traffic 查询采集和 timer，备份恢复，doctor。

## 根因分析

- `sboxctl version` 声明支持 `sing-box|rules` 参数，但组件参数实际返回 `not implemented`。
- `sboxctl doctor` 在 systemd 环境下按 `sbox@INSTANCE.service` 检查实例服务文件，而实际安装契约是单个模板 unit `sbox@.service`。

## 修复方案

- 补齐 `sboxctl version sing-box`，通过受管 `bin/sing-box version` 输出组件版本。
- 补齐 `sboxctl version rules`，输出受管 rules 目录和 `.sbox-manager-managed` 中的文件 hash 摘要。
- 调整 `doctor` 的 systemd 实例服务文件检查路径为 `sbox@.service`，launchd 仍按实例 plist 检查。

## 文件与配置变更

- `internal/cli/common.go`
- `internal/cli/version_component.go`
- `internal/cli/command_test.go`
- `internal/diagnostics/diagnostics.go`
- `internal/diagnostics/diagnostics_test.go`

## 验证结果

- 本地：`go test ./...` 通过。
- 远程第一轮：`/opt/sbox-manager-test-20260628160520/report.log`，复现组件 version 未实现和 doctor systemd 模板判断问题。
- 远程复测：`/opt/sbox-manager-retest-20260628162118/report.log`，270 项，0 失败。
- 远程复测使用代理 `http://user:nopass@10.2.1.107:9605` 完成 `install/update/setup` 默认远端资源下载。
- traffic 采集类命令使用本次上传的 V2Ray Stats gRPC 测试端点验证 CLI、SQLite、查询、导出、timer 逻辑；官方 sing-box 服务启动和 ipinfo 使用真实 sing-box 路径验证。

## 风险与后续

- `rules` 版本输出依赖受管 marker；非本工具安装的 rules 目录会按缺少管理标记报错。
- 官方 sing-box 1.13.14 构建不包含 v2ray API tag，启用 stats API 后不能用于真实 traffic 采集；当前复测已将服务启动和 stats endpoint 分离验证。
