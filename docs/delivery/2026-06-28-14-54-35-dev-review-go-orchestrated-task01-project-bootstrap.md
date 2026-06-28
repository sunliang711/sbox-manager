# T01 项目骨架与 CLI 交付记录

## 任务背景

- 目标：建立 Go 项目骨架，提供 `sboxctl` 和 `sboxsub` 两个二进制入口。
- 范围：初始化 Go module、创建 Cobra CLI 根命令、接入 Zerolog 基础日志上下文、提供 `version` 命令、补齐 T01 所需全局参数与命令树占位。
- 边界：未实现 T02 之后的配置模型、runtime、订阅服务、traffic 业务逻辑；占位命令执行时返回 `not implemented`，不产生运行时目录或服务副作用。

## 多 Agent 编排

- 开发 agent：实现 T01 Go 骨架、CLI 命令树、版本包和测试。
- review agent：独立审查 T01 diff，首轮发现 2 个阻断问题。
- 开发 agent 复修：补齐 `traffic export --date` 和 `sboxsub --listen` 校验，并补充回归测试。
- review agent 复审：阻断问题清零，T01 通过。

## 实现摘要

- 新增 `go.mod`、`go.sum`，module 为 `github.com/sunliang711/sbox-manager`。
- 新增 `cmd/sboxctl/main.go` 与 `cmd/sboxsub/main.go`，入口只调用内部 runner。
- 新增 `internal/version`，支持 Makefile 通过 `-ldflags` 注入 `Version`、`Commit`、`BuildTime`。
- 新增 `internal/cli`，统一构建 `sboxctl` 与 `sboxsub` 的 Cobra 命令树。
- 根命令支持 `--base-dir` 与 `--service-manager auto|systemd|launchd`；`sboxsub` 额外支持并校验 `--listen HOST:PORT`。
- `version` 命令输出版本、commit、build time；其他后续任务命令为无副作用占位。
- 更新 `docs/PROGRESS.md`，将 T01 标记为已完成。

## 评审问题与处理

| 问题 | 处理结果 |
| --- | --- |
| `traffic export hourly|daily|monthly` 缺少 `--date`，与 CLI 规格不一致 | 已补充 `--date` 参数和回归测试 |
| `sboxsub --listen` 未校验 `HOST:PORT` | 已补充格式与端口范围校验，并增加回归测试 |

## 验证结果

- `go test ./...` 通过。
- `make help` 通过。
- `make build` 通过。
- 抽测 `sboxctl version` 和 `sboxsub version`，均输出 `Version`、`Commit`、`BuildTime`。
- 抽测两个根命令 help，能够区分 agent 与订阅服务职责。
- 复审验证阻断问题为“无”。

## 风险与后续建议

- 当前命令树中除 `version` 外均为 T01 占位实现，后续任务需要逐步替换为真实业务逻辑。
- 后续补齐 CLI 业务实现时，建议增加命令树和 flag 的规格校验测试，降低与 `docs/cli-spec.md` 漂移的风险。
