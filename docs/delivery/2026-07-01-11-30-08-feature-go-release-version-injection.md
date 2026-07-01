# release version 注入交付说明

## 任务背景

- 确保 `sboxctl version` 和 `sboxsub version` 可用于查看二进制版本信息。
- GitHub Actions release 构建时按 tag 注入版本号，并同时注入 commit 与构建时间。
- 参考 `../proxystack-go` 的 tag 构建元信息注入方式。

## 实现方案

- 保留现有 `sboxctl version`、`sboxsub version` 子命令。
- 在 release workflow 中增加 tag 语义校验。
- 在 build job 中计算 `build_commit` 和 `build_time`，并通过 `make package` 显式传入 `VERSION="${GITHUB_REF_NAME}"`、`COMMIT`、`BUILD_TIME`。
- 补充验收测试，锁定 workflow 中的版本注入契约。

## 文件变更

- 修改 `.github/workflows/release.yml`：增加 tag 校验与构建元信息注入。
- 修改 `internal/acceptance/release_install_test.go`：补充 release workflow 静态契约断言。
- 修改 `docs/release.md`：补充 GitHub Actions 构建元信息说明。

## 配置与依赖变更

- 无新增配置项。
- 无新增 Go 依赖。
- 无数据库或外部服务变更。

## 测试结果

- `go test ./internal/acceptance` 通过。
- `go test ./internal/cli ./internal/version` 通过。
- `go test ./...` 通过。
- 临时 `DIST_DIR` 执行 `make build VERSION=v9.8.7 COMMIT=abc1234 BUILD_TIME=2026-07-01T03:30:00Z` 后，`sboxctl version` 和 `sboxsub version` 均输出注入值。

## 风险与后续建议

- 当前 tag 校验为严格 `vX.Y.Z`，如果后续需要 `vX.Y.Z-rc.1` 等预发布 tag，需要同步调整 workflow 和验收测试。
