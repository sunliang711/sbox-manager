# sboxctl setup 阶段化改造交付记录

## 背景

- 原 `sboxctl init` 名称过于抽象，用户无法直接判断它是创建目录和配置。
- 原 `setup --start` 容易让用户误以为初始化后可以立即启动；实际刚创建配置后通常还没有 instance。
- 运行资源下载缺少实时进度，重复执行 setup 时也缺少明确跳过行为。

## 实现方案

- 移除 agent 侧顶层 `sboxctl init` 入口，改为 `sboxctl setup local`。
- 将 `sboxctl setup` 改为阶段化命令：
  - `setup local`：创建 base-dir、默认配置、受管目录，安装实例 service 文件和 traffic timer 文件，并启用 traffic timer。
  - `setup binary`：下载并安装 sing-box、geosite.db、geoip.db。
  - `setup all`：依次执行 `local` 和 `binary`。
  - `setup`：等价于 `setup all`。
- 删除 `setup --start` 语义，实例启动统一使用 `sboxctl start [TARGET]`。
- 资源安装器增加远端下载分块进度输出。
- `install` 模式下增加受管 hash 匹配跳过：
  - sing-box 新 marker 记录 payload hash 和 source hash。
  - geosite/geoip 根据受管 marker 中的文件 hash 跳过。
- `service install` 和 `traffic timer install` 在写完 timer 文件后自动 enable traffic timer。

## 文件变更

- `internal/cli/sboxctl.go`：移除 agent 侧 `init` 注册，给 `setup` 挂载阶段参数。
- `internal/cli/sboxctl_t04.go`：实现 `setup local/binary/all`。
- `internal/cli/sboxctl_t06.go`：`traffic timer install` 自动 enable。
- `internal/install/installer.go`：下载进度、安装跳过、sing-box marker 扩展。
- `internal/service/install.go`：systemd 模板在无 instance 时也可安装。
- `internal/diagnostics/diagnostics.go`：traffic timer 缺失提示改为 `setup local`。
- `README.md`、`docs/cli-spec.md` 等文档同步新命令语义。

## 测试结果

- 本地 `go test ./...` 通过。
- 本地交叉编译 `linux/amd64` 通过，远程测试机最终安装 BuildTime `2026-06-29T06:22:32Z` 的 dirty build。
- 远程 smoke 通过：`/opt/sbox-setup-stages-20260629142002-report.log`。
  - `sboxctl setup --help` 展示 `local`、`binary`、`all`。
  - `sboxctl init` 已移除，返回 unknown command。
  - `setup local` 可重复执行，并启用 `sbox-traffic-hourly.timer`。
  - `setup binary` 首次下载 sing-box、geosite.db、geoip.db 时输出实时进度。
  - 第二次 `setup binary` 输出 `skip sing-box already installed` 和 `skip rules already installed`。
  - `uninstall all --purge` 已清理临时 base-dir 和 traffic timer。
- 远程轻量复测通过：`/opt/sbox-setup-help-20260629142252-report.log`。
  - `setup binary --help` 不再展示 `--external-host`、`--force`。
  - 最新构建的 `setup local --external-host` 可正常执行并清理。
- 已新增/更新测试覆盖：
  - `setup local` 创建配置、安装 service/timer 并 enable timer。
  - `setup` 默认执行 local 后执行 binary。
  - `service install` 不启动实例服务，但会 enable traffic timer。
  - sing-box 与 geosite/geoip 已安装时跳过重复安装。
  - 远端下载输出 progress。

## 风险与后续

- 已安装旧版 sing-box marker 只记录 payload hash，不记录 source hash；升级到新逻辑后首次执行 `setup binary` 可能需要再下载一次，之后即可按 source hash 跳过。
- `setup local` 会无条件 enable traffic timer；如果尚未配置可采集实例，后续 timer 触发时可能产生“没有可采集的 traffic instance”的日志，但不会启动实例服务。
