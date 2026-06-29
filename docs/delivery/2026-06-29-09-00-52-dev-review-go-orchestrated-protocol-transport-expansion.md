# 协议与 Transport 扩展独立评审交付说明

## 任务背景

在完成 VMess/VLESS transport、VLESS、AnyTLS 和 Surge 订阅过滤扩展后，按用户要求启动独立 agent 对当前未提交改动进行 Go 代码评审，并由主 agent 修复评审发现的问题。

## 编排方案

- 主 agent 整理当前变更范围、测试结果和已知风险。
- 独立 review agent `Kierkegaard` 审查未提交改动，仅输出问题，不修改文件。
- 主 agent 根据评审结论修复阻断和警告问题。
- 修复后执行针对性测试和全量测试。

## 评审问题与处理结果

| 等级 | 问题 | 处理结果 |
| --- | --- | --- |
| 阻断 | VMess/VLESS 的 `network` 与 V2Ray `transport` 混用，可能生成无效 sing-box 配置。 | 已将 `network` 限定为 VMess 的 `tcp/udp` 底层网络，删除 `network -> transport.type` 映射，订阅节点不再把 transport 写入 `network`。 |
| 阻断 | AnyTLS outbound/subscription node 可缺少 TLS，但 sing-box AnyTLS outbound 要求 TLS。 | 已在校验层要求 AnyTLS inbound/outbound/subscription node 启用 TLS，并在生成层兜底输出 `tls.enabled=true`。 |
| 警告 | VMess/VLESS user 字段共用转换分支，可能把 `flow` 输出到 VMess 或把 `alterId` 输出到 VLESS。 | 已拆分 VMess/VLESS 用户转换，并在校验层拒绝跨协议字段。 |

## 文件与配置变更

- `internal/domain/validation.go`
- `internal/domain/validation_test.go`
- `internal/generator/singbox/generator.go`
- `internal/generator/singbox/subscription.go`
- `internal/generator/singbox/generator_test.go`
- `internal/subscription/render.go`
- `internal/subscription/render_test.go`
- `docs/data-spec.md`

## 测试结果

- `go test ./internal/domain ./internal/generator/singbox ./internal/subscription ./internal/subserver ./internal/cli`
- `go test ./...`

两组测试均通过。

## 风险与后续建议

- Surge 官方文档仅列出 `ws-headers` 参数名，未展开具体格式，仍建议后续用真实 Surge 配置导入做人工验证。
- Clash/Premium Clash 仍保持既有支持范围，未在本次 review 修复中扩展 VLESS/AnyTLS。
