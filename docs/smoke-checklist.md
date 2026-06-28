# 真实 Smoke 验收清单

本文档记录 T08 之后需要在真实 Linux/systemd 与 macOS/launchd 环境中执行的 smoke 验收。普通自动化测试不得依赖 root、真实 systemd、真实 launchd、真实网络或真实 sing-box；以下步骤只在显式准备好的验收主机上执行。

## 前置条件

- 已安装本项目构建出的 `sboxctl` 和 `sboxsub`。
- 已准备独立测试 base dir，避免覆盖生产配置。
- 已准备可用 sing-box binary 和规则资源。
- 执行前记录 OS、架构、二进制版本、commit 和 build time。

## Linux/systemd

### Instance Service

- `sboxctl --base-dir <agent-dir> init --external-host <host> --force`
- `sboxctl --base-dir <agent-dir> add edge-smoke --no-edit`
- `sboxctl --base-dir <agent-dir> service install edge-smoke`
- `systemctl cat sbox@edge-smoke.service`，确认 `ExecStart`、`WorkingDirectory`、日志和 hardening 字段。
- `sboxctl --base-dir <agent-dir> start edge-smoke`
- `sboxctl --base-dir <agent-dir> status edge-smoke`
- `sboxctl --base-dir <agent-dir> logs edge-smoke`
- `sboxctl --base-dir <agent-dir> stop edge-smoke`
- `sboxctl --base-dir <agent-dir> service uninstall edge-smoke`

### Subscription Service

- `sboxsub --base-dir <sub-dir> init --force`
- `sboxsub --base-dir <sub-dir> service install`
- `systemctl cat sboxsub.service`，确认 `ExecStart` 使用 `sboxsub --base-dir <sub-dir> serve`。
- `sboxsub --base-dir <sub-dir> start`
- `sboxsub --base-dir <sub-dir> status`
- `sboxsub --base-dir <sub-dir> logs`
- `sboxsub --base-dir <sub-dir> stop`
- `sboxsub --base-dir <sub-dir> service uninstall`

### Traffic Timer

- `sboxctl --base-dir <agent-dir> traffic timer install`
- `systemctl cat sbox-traffic-hourly.service`
- `systemctl cat sbox-traffic-hourly.timer`
- `systemctl cat sbox-traffic-daily.service`
- `systemctl cat sbox-traffic-daily.timer`
- `systemctl cat sbox-traffic-monthly.service`
- `systemctl cat sbox-traffic-monthly.timer`
- `sboxctl --base-dir <agent-dir> traffic timer enable`
- `sboxctl --base-dir <agent-dir> traffic timer status`
- `sboxctl --base-dir <agent-dir> traffic timer logs`
- `sboxctl --base-dir <agent-dir> traffic timer run hourly`
- `sboxctl --base-dir <agent-dir> traffic timer run daily`
- `sboxctl --base-dir <agent-dir> traffic timer run monthly`
- `sboxctl --base-dir <agent-dir> traffic timer disable`
- `sboxctl --base-dir <agent-dir> traffic timer uninstall`

## macOS/launchd

### Instance Service

- `sboxctl --base-dir <agent-dir> init --external-host <host> --force`
- `sboxctl --base-dir <agent-dir> add edge-smoke --no-edit`
- `sboxctl --base-dir <agent-dir> service install edge-smoke`
- `plutil -lint ~/Library/LaunchAgents/com.sbox-manager.edge-smoke.plist`
- `sboxctl --base-dir <agent-dir> start edge-smoke`
- `sboxctl --base-dir <agent-dir> status edge-smoke`
- `sboxctl --base-dir <agent-dir> logs edge-smoke`
- `sboxctl --base-dir <agent-dir> stop edge-smoke`
- `sboxctl --base-dir <agent-dir> service uninstall edge-smoke`

### Subscription Service

- `sboxsub --base-dir <sub-dir> init --force`
- `sboxsub --base-dir <sub-dir> service install`
- `plutil -lint ~/Library/LaunchAgents/com.sbox-manager.sboxsub.plist`
- `sboxsub --base-dir <sub-dir> start`
- `sboxsub --base-dir <sub-dir> status`
- `sboxsub --base-dir <sub-dir> logs`
- `sboxsub --base-dir <sub-dir> stop`
- `sboxsub --base-dir <sub-dir> service uninstall`

### Traffic Timer

- `sboxctl --base-dir <agent-dir> traffic timer install`
- `plutil -lint ~/Library/LaunchAgents/com.sbox-manager.traffic.hourly.plist`
- `plutil -lint ~/Library/LaunchAgents/com.sbox-manager.traffic.daily.plist`
- `plutil -lint ~/Library/LaunchAgents/com.sbox-manager.traffic.monthly.plist`
- `sboxctl --base-dir <agent-dir> traffic timer enable`
- `sboxctl --base-dir <agent-dir> traffic timer status`
- `sboxctl --base-dir <agent-dir> traffic timer logs`
- `sboxctl --base-dir <agent-dir> traffic timer run hourly`
- `sboxctl --base-dir <agent-dir> traffic timer run daily`
- `sboxctl --base-dir <agent-dir> traffic timer run monthly`
- `sboxctl --base-dir <agent-dir> traffic timer disable`
- `sboxctl --base-dir <agent-dir> traffic timer uninstall`

## 通过标准

- 服务文件只写入对应 systemd unit/timer 或 launchd plist。
- `start/status/logs/stop` 均能返回预期状态或清晰错误。
- traffic timer `install/enable/disable/status/logs/run/uninstall` 均可独立执行。
- 卸载后受管服务文件被删除，配置、traffic DB 和日志按命令语义保留。
- 任一步失败时记录命令、输出、退出码、系统版本和当前配置摘要。
