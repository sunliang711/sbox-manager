# T02 配置模型与校验交付记录

## 任务背景

- 目标：实现新项目配置 schema、领域模型、默认值和校验逻辑。
- 范围：agent 全局配置、instance 配置、sboxsub 配置、traffic 配置、订阅 input、bundle manifest、backup manifest 的严格加载骨架和核心校验。
- 边界：未实现 T03 之后的 sing-box 生成、runtime 写入、订阅 HTTP 服务或 traffic DB 仓储。

## 多 Agent 编排

- 开发 agent：实现 `internal/domain` 与 `internal/config` 的模型、默认值、严格解码、路径清理、端口范围解析和校验测试。
- review agent：独立审查 T02 实现，首轮发现 1 个阻断问题。
- 开发 agent 复修：补齐严格解码的 EOF/单文档校验，并增加回归测试。
- review agent 复审：阻断问题清零，T02 通过。

## 实现摘要

- 新增 `internal/domain`：
  - `GlobalConfig`、`Instance`、`Inbound`、`Outbound`、`Group`、`RouteRule`、`SubConfig`、`TrafficConfig` 等核心模型。
  - 默认配置构造函数。
  - 聚合校验错误类型。
  - 默认配置、安全鉴权、引用关系、端口冲突、不支持类型等校验逻辑。
- 新增 `internal/config`：
  - 严格 YAML/JSON 解码，拒绝未知字段、多 YAML document 和 JSON trailing token。
  - 全局配置、instance、sub 配置、traffic 配置、订阅 input、订阅 index、bundle manifest、backup manifest 加载入口。
  - 路径规范化与路径穿越拒绝。
  - 端口范围 `start-end` 和 `{start,end}` 解析，以及简单端口分配。
- `go.mod` / `go.sum` 新增 `gopkg.in/yaml.v3`，用于严格 YAML 解码。
- `docs/PROGRESS.md` 将 T02 标记为已完成。

## 评审问题与处理

| 问题 | 处理结果 |
| --- | --- |
| 严格解码只读取第一份 YAML/JSON 顶层值，可能忽略后续 document 或 trailing object | 已在解码后继续读取并要求 EOF；补充 YAML 多文档和 JSON 尾随对象回归测试 |

## 验证结果

- `go test ./...` 通过。
- `make build` 通过。
- review 复审验证阻断问题为“无”。

## 风险与后续建议

- 当前 `SubscriptionInput`、`BundleManifest`、`BackupManifest` 以严格解码骨架为主，完整 hash、路径集合和导入原子性将在后续订阅/备份任务中实现。
- 后续可继续补充端口冲突组合测试，例如 inbound vs API、API vs API、disabled instance 跳过。
