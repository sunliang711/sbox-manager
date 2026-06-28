# sboxctl / sboxsub 远程全量 Smoke 修复记录

## 问题背景

- 测试环境：`root@10.2.100.157`，Debian GNU/Linux 13 amd64，systemd。
- 构建方式：本地使用 Zig 作为 Linux CGO 交叉编译器，生成 `linux/amd64` 静态二进制后上传测试机。
- 测试范围：`sboxctl` 与 `sboxsub` 根命令、嵌套子命令 help、配置初始化、实例管理、订阅导入导出、服务生命周期、资源安装、traffic SQLite/timer、备份恢复、HTTP 订阅服务与 token 鉴权。

## 根因分析

- `sboxctl init`、`sboxsub init` 和 `sboxctl add/clone` 生成的配置文件没有注释，远程可用性检查不通过。
- `sboxctl clone` 对启用 stats API 的实例只重分配 inbound 端口，未重分配 API 端口，导致克隆实例与源实例 API 端口冲突。
- `sboxsub input clone` 原样复制 input 后保留旧 `source`、node `id`、`tag`、`remark`，即使不编辑也会因重复约束校验失败。

## 修复方案

- agent 全局配置和 instance 配置写入时增加字段说明、模板说明和订阅片段示例注释。
- sboxsub 默认 `config.yaml` 增加 listen、access、templates、watch、managed config 等配置项注释。
- clone 分配端口时将已启用的 API 端口纳入占用集合，并为目标实例重分配 API 端口。
- input clone 在打开 editor 前按目标文件名重写 `source`、node `id`、`tag` 和 `remark`，确保默认克隆结果可直接通过整体验证。

## 验证结果

- 本地：`go test ./...` 通过。
- 远程：`/opt/sbox-manager-test-20260628222256/report.log`，135 项，0 失败。
- 远程同时验证了 `setup` 通过代理下载 sing-box、geosite、geoip 并完成 sha256 校验。

## 风险与后续

- 当前注释以文件头和关键配置项说明为主，不改变 YAML schema。
- `sboxsub input clone` 会为自定义 remark 添加目标 source 前缀，避免同一 user 下展示名重复。
