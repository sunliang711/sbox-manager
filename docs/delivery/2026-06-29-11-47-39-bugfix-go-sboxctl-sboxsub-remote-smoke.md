# sboxctl / sboxsub 远程全量 Smoke 修复记录

## 问题背景

- 测试环境：`root@10.2.100.157`，Linux amd64，systemd。
- 构建方式：本地交叉编译 `linux/amd64` 后上传并安装到 `/usr/local/bin/sboxctl`、`/usr/local/bin/sboxsub`。
- 测试范围：`sboxctl`、`sboxsub` 全部子命令 help，常用分组命令直接执行体验，配置初始化注释，实例模板注释，订阅导出导入，HTTP 订阅服务，systemd 生命周期，资源安装更新卸载，traffic 查询和 timer，备份恢复，doctor。

## 根因分析

- `sboxctl render` 是分组命令，但直接执行时返回 `not implemented: sboxctl render`。
- 其他分组命令如 `sboxctl member`、`sboxctl service`、`sboxctl traffic`、`sboxsub input` 直接执行时都会展示帮助；`render` 的行为不一致，增加了新用户理解成本。
- `sboxctl setup --start` 内部复用 `start` 命令时传入 nil 参数，Cobra 会回退解析进程原始参数，导致内部 `start` 报 `unknown flag: --base-dir`。

## 修复方案

- 将 `sboxctl render` 分组命令的默认执行行为改为 `cmd.Help()`。
- `setup --start` 内部复用生命周期命令时，将 nil 参数转换为空切片，避免重新解析外层 root flags。
- 本地化 Cobra help 模板中的固定标题和默认 help 文案，将 `Usage`、`Flags`、`Global Flags` 等改为 `用法`、`选项`、`全局选项`，降低中文用户阅读成本。
- 优化订阅导出和 traffic 查询的人类可读输出：`sub export --dry-run/--summary` 输出中文摘要，traffic 空结果输出 `未找到记录`，traffic 摘要上下文字段使用中文标签。
- 增加回归测试，确保直接执行 `sboxctl render` 时展示帮助而不是报未实现，`setup --start` 不会误解析 `--base-dir`，根命令 help 和关键查询输出保持中文可读。

## 验证结果

- 本地：`go test ./...` 通过。
- 远程第一轮：`/opt/sbox-smoke-20260629114114/report.log`，252 项中发现 1 个代码体验问题和 4 个测试脚本断言/顺序问题。
- 远程回归：`/opt/sbox-smoke-retest-20260629114645/report.log`，252 项，252 通过，0 失败；其中 3 项为依赖真实代理或 stats gRPC endpoint 的预期失败路径，错误提示清晰。
- 远程 setup：使用代理 `http://user:nopass@10.2.1.107:9605` 执行默认下载版 `sboxctl setup`，sing-box 和 rules 下载、sha256 校验、安装、doctor、`uninstall all --purge` 均通过；日志：`/opt/sbox-setup-smoke-20260629114719-report.log`。
- 远程 traffic stats 成功路径：使用临时 V2Ray Stats gRPC endpoint 验证 `traffic check health`、`traffic show current`、`traffic watch current`、`traffic collect hourly|daily|monthly` 和对应 show 命令均通过；日志：`/opt/sbox-traffic-stats-smoke-20260629115203-report.log`。
- 远程 setup --start 与 ipinfo 成功路径：使用代理下载官方 sing-box/rules，`sboxctl setup --start` 成功启动实例，`sboxctl ipinfo edge-smoke --family ipv4` 通过真实本地 socks5 代理返回 IPv4，随后 `stop` 和 `uninstall all --purge` 清理成功；日志：`/opt/sbox-setup-start-smoke-20260629115604-report.log`。
- 远程可读性回归：交叉编译并安装 BuildTime `2026-06-29T04:06:34Z` 的 linux/amd64 二进制，验证 `sboxctl --help`、`sboxctl render --help`、`sboxsub --help`、`sboxsub input --help`、`sub export --dry-run/--summary`、`traffic check config`、`traffic show hourly`、`traffic summarize hourly` 的输出可读性，临时 base-dir 已通过 `sboxctl uninstall all --purge` 清理；日志：`/opt/sbox-readable-smoke-20260629120723-report.log`。

## 风险与后续

- 本次仅修改 `sboxctl render` 分组默认行为，不影响 `render model`、`render sing-box`、`render sub` 子命令。
- `ipinfo` 成功路径依赖测试机出站网络，本次在 `root@10.2.100.157` 已通过官方 sing-box 本地 socks5 代理验证。
- 表格字段名和 CSV/JSON/YAML 等机器可读输出保持原样，避免破坏已有脚本；本次优先调整 help 标题、摘要、空结果和状态提示等面向人的文本。
