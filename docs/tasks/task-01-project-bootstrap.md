# T01 项目骨架与 CLI

## 目标

建立 Go 项目骨架，提供 `sboxctl` 和 `sboxsub` 两个二进制入口。

## 范围

- 初始化 Go module。
- 创建 `cmd/sboxctl` 和 `cmd/sboxsub`。
- 创建 `internal/version`。
- 接入 Cobra 根命令。
- 接入 Zerolog 基础日志。
- 提供 `version` 命令。
- 接入全局 `--base-dir` 和 `--service-manager auto|systemd|launchd` 参数。

## 技术方案

目录按 `docs/conventions.md` 规划创建。`cmd/*/main.go` 只调用 runtime runner，不承载业务逻辑。

## 验收标准

- `go test ./...` 通过。
- `sboxctl version` 输出版本、commit、build time。
- `sboxsub version` 输出版本、commit、build time。
- 根命令 help 中清楚区分 agent 和 sub 职责。
- help 命令展示的子命令结构与 `docs/cli-spec.md` 一致。
- 不创建任何运行时目录。

## 风险

- 过早引入过多依赖。初始项目骨架只引入 CLI、日志、测试必要依赖。
