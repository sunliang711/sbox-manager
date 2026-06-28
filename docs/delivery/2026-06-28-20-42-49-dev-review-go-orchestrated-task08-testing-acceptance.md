# T08 测试与验收交付记录

## 结论

- 状态：已完成，并通过独立 review agent 复审，最终结论为“无问题”。
- 范围：自动化测试补强、release/install 验收测试、CLI fake e2e、真实 smoke checklist。
- 边界：普通自动化测试不依赖 root、真实 systemd、真实 launchd、真实网络或真实 sing-box；真实环境 smoke 以 checklist 形式独立执行。

## 主要实现

- generator golden：
  - 新增 `internal/generator/singbox/testdata/edge-us.golden.json`。
  - `TestGenerateMatchesGolden` 固定 edge 实例生成 JSON 的字节级输出。
- acceptance 测试：
  - 新增 `internal/acceptance/release_install_test.go`。
  - 静态检查 release workflow 的 tag 触发、权限、四平台矩阵、checksum 和 release 上传资产。
  - 验证 `install.sh --dry-run`、`install-local.sh --dry-run` 不写目标目录。
  - 验证 `install-local.sh` 拒绝路径穿越归档。
  - 验证 `install.sh` 默认执行 checksum 校验，checksum mismatch 时不安装二进制。
- CLI 验收：
  - `validate/render/doctor` 只读副作用测试，同时断言命令输出或预期错误。
  - `init/add/check/start/status/logs/stop` fake e2e，使用 fake service manager，避免触碰真实 systemd。
- smoke 文档：
  - 新增 `docs/smoke-checklist.md`。
  - 覆盖 Linux/systemd 与 macOS/launchd 的 instance service、subscription service、traffic timer。
  - traffic timer checklist 包含 hourly/daily/monthly 的 install/enable/disable/status/logs/run/uninstall。

## Review 修复

- 只读测试不再丢弃命令错误，避免“失败但无写入”误判为通过。
- release workflow 静态检查改为约束完整矩阵组合和 release 资产命令。
- 安装脚本补充 checksum mismatch 不安装的真实非 dry-run 测试路径。
- smoke checklist 补齐 daily/monthly traffic timer 检查和 run 步骤。

## 验证

- `go test ./internal/generator/singbox`
- `go test ./internal/acceptance`
- `go test ./internal/cli`
- `go test ./internal/acceptance ./internal/cli`
- `go test ./...`
- `go test -race ./internal/cli ./internal/service ./internal/traffic ./internal/subserver`
- `go mod tidy -diff`

说明：macOS race 测试期间出现系统 linker `LC_DYSYMTAB` warning，但测试返回码为 0。
