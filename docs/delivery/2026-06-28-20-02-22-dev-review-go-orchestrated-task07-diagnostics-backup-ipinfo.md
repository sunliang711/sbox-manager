# T07 诊断、ipinfo、备份交付记录

## 结论

- 状态：已完成，并通过独立 review agent 复审，最终结论为“无问题”。
- 范围：`sboxctl doctor`、`sboxctl ipinfo`、`sboxctl export/import`、`sboxsub doctor`、`sboxsub config show --show-secrets` 回归验证。
- 边界：真实出口 IP 查询依赖外部 endpoint 和本地 sing-box listener，部署验收阶段仍需在真实实例上做 smoke test。

## 主要实现

- 新增 `internal/backup`：
  - 新格式 agent backup zip，固定 `backup_manifest.json`。
  - manifest schema/version/hash 校验。
  - 仅包含 `config.yaml` 和 `instances/*.yaml|*.yml|*.json`。
  - 拒绝订阅 bundle、旧 native backup、未知成员和路径穿越成员。
  - import 支持 `--force` 覆盖策略。
- 新增 `internal/diagnostics`：
  - doctor 输出 `OK`/`ISSUE`，发现 ISSUE 返回非零。
  - 检查目录、sing-box binary、实际 `sing-box check -c`、服务文件、监听端口、traffic DB/timer、sub inputs/service。
  - ipinfo 通过实例 socks5/http listener 查询出口 IP，支持 `all|ipv4|ipv6`、timeout 和 endpoint fallback。
- 接入 CLI：
  - `sboxctl export/import/doctor/ipinfo`。
  - `sboxsub doctor` 使用诊断模块并在 ISSUE 时返回非零。
  - `sboxsub config show` 默认脱敏，`--show-secrets` 显示明文。

## Review 修复

- import 拒绝 backup 内 `paths.instances` 指向 `base-dir` 外部，防止写出受控目录。
- import 改为先预写全部临时文件、再成批替换目标文件；失败时回滚，且 `config.yaml` 最后替换。
- doctor 的 `sing-box.check` 从只检查可执行位改为实际执行 `sing-box check -c`。
- 补充外部路径拒绝、导入失败不替换旧 config、真实执行 `sing-box check` 的回归测试。

## 验证

- `go test ./internal/backup`
- `go test ./internal/diagnostics`
- `go test ./...`
- `go test -race ./internal/backup ./internal/diagnostics ./internal/cli`
- `go test -race ./internal/traffic ./internal/service`
- `go mod tidy -diff`

说明：macOS race 测试期间出现系统 linker `LC_DYSYMTAB` warning，但测试返回码为 0。
