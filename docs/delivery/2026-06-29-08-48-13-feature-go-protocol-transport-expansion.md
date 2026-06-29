# 协议与 Transport 扩展交付说明

## 任务背景

当前项目需要扩充 sing-box 协议覆盖范围，重点支持 VMess/VLESS 的 V2Ray transport、VLESS 多方式配置、AnyTLS 协议，以及 Surge 订阅对不支持协议的跳过逻辑。

## 实现方案

- 领域模型新增 `TransportConfig`，覆盖 sing-box 当前 V2Ray transport：`http`、`ws`、`quic`、`grpc`、`httpupgrade`。
- Inbound/Outbound/SubscriptionNode 新增 VLESS/AnyTLS 所需字段：`tls`、`transport`、`flow`、`alter_id`、`security`。
- sing-box 生成器支持 VMess/VLESS inbound/outbound transport，支持 AnyTLS inbound/outbound。
- 订阅 input 从 inbound 导出时保留 VLESS/AnyTLS、TLS、transport、flow、alterId。
- Surge 渲染支持 VMess WebSocket、VMess AEAD、AnyTLS，并跳过 VLESS。

## 文件变更

- `internal/domain/models.go`
- `internal/domain/validation.go`
- `internal/generator/singbox/model.go`
- `internal/generator/singbox/generator.go`
- `internal/generator/singbox/subscription.go`
- `internal/subscription/render.go`
- `internal/domain/validation_test.go`
- `internal/generator/singbox/generator_test.go`
- `internal/subscription/render_test.go`
- `docs/data-spec.md`
- `README.md`

## 参考资料

- sing-box V2Ray Transport: `https://sing-box.sagernet.org/configuration/shared/v2ray-transport/`
- sing-box VLESS inbound/outbound: `https://sing-box.sagernet.org/configuration/inbound/vless/`、`https://sing-box.sagernet.org/configuration/outbound/vless/`
- sing-box AnyTLS inbound/outbound: `https://sing-box.sagernet.org/configuration/inbound/anytls/`、`https://sing-box.sagernet.org/configuration/outbound/anytls/`
- Surge Proxy Policy: `https://manual.nssurge.com/policy/proxy.html`

## 测试结果

- `go test ./internal/domain ./internal/generator/singbox ./internal/subscription ./internal/subserver`
- `go test ./...`

两组测试均通过。

## 风险与后续建议

- Surge 官方文档列出 `ws-headers` 参数但未展开格式，本次按稳定的 `key:value|key:value` 形式输出。
- Clash/Premium Clash 仍保持既有订阅过滤策略，后续如需完整支持 VLESS/AnyTLS，可单独扩展模板字段。
