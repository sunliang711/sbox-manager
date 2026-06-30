# sboxctl add/example 同源实例模板

## 背景

`sboxctl example` 和 `sboxctl add` 之前分别维护协议示例和实例生成内容，新增或调整协议字段时容易出现两边不一致。

## 实现方案

- 新增 `internal/configtemplate`，用 `embed.FS` 管理共享 YAML 片段。
- 将 inbound、outbound、group、route、transport 拆成可独立渲染的 snippet。
- `sboxctl example inbound/outbound/group/route` 改为从同一份 snippet registry 渲染。
- `sboxctl add --template edge|relay|urltest` 的生效 instance 正文改为由完整 instance 模板 include 共享 snippet 生成。
- `add` 文件尾部的协议模板参考也改为由共享 snippet 生成注释块。

## 文件变更

- 新增：`internal/configtemplate/`
- 修改：`internal/instance/manager.go`
- 修改：`internal/cli/sboxctl_t04.go`
- 修改：`internal/instance/manager_test.go`
- 修改：`internal/cli/command_test.go`

## 行为保持

- `edge`、`relay`、`urltest` 的默认端口、订阅、route 和 ref member 语义保持不变。
- `clone`、`member add/remove` 仍使用现有强类型写回路径，避免覆盖用户自定义实例结构。
- `example inbound vmess`、`example outbound vmess` 等历史调用保留 alias，同时新增 `vmess-raw`、`vmess-websocket`、`vmess-grpc`、`vless-websocket`、`vless-grpc` 等细分模板。

## 测试

- `go test ./internal/configtemplate ./internal/instance ./internal/cli`
- `go test ./...`

## 风险与后续

- 当前同源模板已覆盖内置 `add` 的新增实例正文和协议参考；已有实例经 `member` 写回仍走旧强类型 marshal，避免本次扩大影响范围。
- 后续如需要，可以继续把 member 写回也接入模板化输出，但需要额外设计保留用户自定义字段的策略。
