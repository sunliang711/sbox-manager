# T08 测试与验收

## 目标

建立覆盖三大功能的自动化测试和端到端验收。

## 范围

- 单元测试。
- golden 测试。
- CLI 副作用测试。
- HTTP server 测试。
- traffic fixture 测试。
- systemd/launchd fake 测试。
- e2e dry-run 测试。
- Linux/systemd 和 macOS/launchd 真实 smoke checklist。

## 技术方案

优先用 fake filesystem、fake service manager、fixture stats 输出隔离外部依赖。真实 sing-box e2e 可通过环境变量显式开启。

默认自动化测试不要求 root、真实 systemd、真实 launchd 或真实网络；真实 Linux/macOS smoke 作为实现完成后的独立验收清单。

## 验收标准

- `go test ./...` 通过。
- generator golden 稳定。
- `check/render/validate/doctor` 只读行为有测试。
- bundle 路径穿越和 hash mismatch 有测试。
- traffic delta、reset_detected、ALL 小计、动态当前周期有测试。
- HTTP 鉴权和 reload 失败有测试。
- e2e 覆盖 init/add/check/start/status/logs/stop 的 dry-run 路径。
- Linux/systemd 和 macOS/launchd 的 service install/start/status/logs/stop 均有 fake 验收。
- 真实 smoke checklist 覆盖 Linux/systemd 和 macOS/launchd 的 service install/start/status/logs/stop、traffic timer install/enable/disable/status/logs/run/uninstall。

## 风险

- 真实 sing-box 依赖版本可能导致 CI 不稳定。真实 e2e 必须和普通单元测试分离。
