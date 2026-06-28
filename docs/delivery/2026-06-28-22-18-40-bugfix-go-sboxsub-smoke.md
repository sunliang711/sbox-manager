# sboxctl / sboxsub 远程 Smoke 修复记录

## 问题背景

- 测试环境：`root@10.2.151.46`，Debian 13 amd64，systemd。
- 测试范围：`sboxctl sub`、`sboxsub` 全部子命令，补充验证 instance service、resource install/update/uninstall、traffic SQLite/timer。

## 根因分析

- `sboxctl clone` 使用浅拷贝，重新分配端口时会污染源实例的 inbound slice，导致端口冲突误报。
- 默认 edge/urltest 模板的订阅 remark 都来自 inbound 名称，多个默认实例导出 bundle 时会触发同一 user 下 remark 重复。
- systemd unit 固定以 `sbox:sbox` 运行，但 service install 未准备系统用户和目录属主。
- runtime 生成文件在 `sboxctl start/restart` 后为 `root:root 0640`，`sbox` 服务用户无法读取 sing-box 配置。
- Makefile 强制 `CGO_ENABLED=0`，导致依赖 `go-sqlite3` 的 traffic 命令在真实 Linux 二进制中不可用。

## 修复方案

- clone 使用深拷贝复制可变 slice，并把默认实例名订阅 remark 从源实例名更新为目标实例名。
- 默认 edge/urltest 模板的订阅 remark 使用实例名，避免多实例默认导出冲突。
- systemd service/timer install 前确保 `sbox` 用户/组存在，并递归设置 base-dir 属主。
- runtime 原子写入后在 Linux root 场景下将新生成目录和文件 chown 给 `sbox`，保留 `0640` 权限。
- Makefile 改为可配置 `CGO_ENABLED ?= 1`，避免 traffic SQLite 运行时 stub。

## 验证结果

- 本地：`go test ./...` 通过。
- 远程修复项复测：`/opt/sbox-manager-retest-20260628221111/report.log`，52 项，0 失败。
- 远程 lifecycle 本地 source 复测：`/opt/sbox-manager-lifecycle-local-20260628221734/report.log`，20 项，0 失败。
- 真实 GitHub release 下载曾出现 TLS handshake timeout；已用 `--source --sha256` 本地 source 覆盖安装命令语义验证。

## 风险与后续

- `build-linux` 在非 Linux 主机上启用 CGO 后需要可用交叉 C 编译器；本次远程复测使用 Linux 原生构建通过。
- systemd base-dir 建议使用 `/opt` 等非 `/tmp` 路径；`PrivateTmp=true` 下 `/tmp` base-dir 会触发 systemd namespace 限制。
