# T04 实例生命周期与安装交付记录

## 任务

- 任务文档：`docs/tasks/task-04-instance-lifecycle-install.md`
- 开发执行：T04 开发 agent 已启动；主线在其未产出代码后接管实现。
- 独立复审：Feynman

## 交付内容

- 实现实例配置管理：`init`、`setup`、`add`、`example`、`clone`、`member list/add/remove`、`remove`、`config`、`list`。
- 实现生命周期命令：`start/restart` 继续复用 runtime plan、生成配置和 `sing-box check`；`stop/status/logs/enable/disable` 只调用服务管理器。
- 实现 systemd/launchd 服务管理：
  - systemd instance unit：`sbox@<instance>.service`
  - launchd label：`com.sbox-manager.<instance>`
  - `service install` 只写 unit/plist，不启动服务。
  - systemd/launchd 外部命令均通过参数数组执行。
- 实现资源安装器：
  - `install/update/uninstall sing-box|rules|all`
  - 远端 source 强制 sha256。
  - 内置 source 带可信 checksum 元数据。
  - 归档成员路径安全校验。
  - sing-box 更新失败回滚旧二进制。
  - 非受管 symlink 拒绝覆盖。
- 实现 `uninstall --purge`：
  - 默认 uninstall 保留 `config.yaml` 和 `instances/`。
  - purge 删除受管资源目录，并通过 CLI 删除服务文件。

## 复审闭环

首轮复审发现 4 个阻断问题：

- 默认内置 source 缺失，导致 `setup` 默认 `install all` 必然失败。
- `uninstall --purge` 未删除受管目录和服务文件。
- sing-box marker 写入失败时可能已覆盖旧二进制。
- launchd fresh install 后 start/restart/enable 未 bootstrap plist。

修复后复审结论：

- 阻断问题：无

## 验证

- `go test ./...`：通过
- `make build`：通过

## 说明

- 普通测试均使用 fake runner、临时目录或本地 source，不会真实启动/停止本机服务，也不会写入系统服务目录。
- 未做真实默认 source 下载和系统级 service install 端到端演练。
