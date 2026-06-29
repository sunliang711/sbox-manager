# T04 实例生命周期与安装

## 目标

实现实例生命周期管理、服务文件安装、sing-box 安装更新和卸载。

## 范围

- `setup local`
- `setup binary`
- `setup all`
- `add`
- `example`
- `clone`
- `member list/add/remove`
- `remove`
- `config`
- `list`
- `start/stop/restart/status/logs/enable/disable`
- `service install/uninstall/start/stop/restart/status/logs/enable/disable`
- `install/update/uninstall sing-box|rules|all`

不包含本项目自身二进制 release 和安装脚本；该部分由 T09 负责。

## 技术方案

同时支持 Linux/systemd 和 macOS/launchd。`--service-manager auto` 在 Linux 选择 systemd，在 macOS 选择 launchd。

安装器要求：

- 远端下载必须 sha256。
- 内置 source 必须具备可信 checksum 元数据；自定义远端 source 缺少 `--sha256` 时失败。
- 归档成员必须路径安全。
- 更新失败要保留旧二进制。
- 非受管 symlink 不覆盖。

## 验收标准

- 服务名为 `sbox@<instance>.service`。
- launchd label 为 `com.sbox-manager.<instance>`。
- `start` 前执行完整生成和 `sing-box check`。
- `stop/status/logs` 不写 runtime。
- `service install` 在 Linux 写 systemd unit，在 macOS 写 launchd plist，且不启动服务。
- systemd unit 和 launchd plist 的关键字段符合 `docs/data-spec.md`。
- `setup` 按顺序执行 init、install all、service install，`--start` 时继续执行 start。
- `uninstall` 默认保留配置和数据。
- `uninstall --purge` 删除受管目录和服务文件。

## 风险

- 权限不足时行为不清晰。所有权限错误必须明确输出目标路径和操作。
