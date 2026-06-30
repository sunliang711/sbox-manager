# 功能交付：补齐 VMess/VLESS 官方传输组合与 VLESS REALITY Vision

## 任务背景

当前 inbound/outbound 的 VMess/VLESS 模板缺少 HTTP、QUIC、HTTPUpgrade 等官方 sing-box transport 组合，也缺少 VLESS + REALITY + Vision 的模型、生成和模板支持。用户确认采用方案 A：只支持官方 sing-box 兼容能力，不将 Xray 侧 `xhttp` 加入生成器或校验白名单。

## 实现方案

- 扩展领域模型 `TLSConfig`，新增 REALITY 和 uTLS 配置结构。
- 扩展 sing-box 生成器，按 inbound/outbound 区分 REALITY 字段：
  - inbound 输出 `handshake`、`private_key`、`short_id` 列表。
  - outbound 输出 `public_key`、`short_id`，并支持 `utls.fingerprint`。
- 调整 TLS 校验：
  - REALITY inbound 不再要求传统证书路径。
  - REALITY inbound 必须配置握手目标和私钥。
  - REALITY outbound 必须配置公钥。
  - `xhttp` 不进入官方 transport 白名单，仍会被拒绝。
- 新增 VMess/VLESS HTTP、QUIC、HTTPUpgrade、VLESS REALITY Vision inbound/outbound 模板，并接入 `example` 和 `add` 共享协议参考。
- 订阅导出和订阅渲染保留 REALITY 客户端所需公开字段，并对 private key 做脱敏保护。

## 文件变更

- 修改：
  - `internal/domain/models.go`
  - `internal/domain/validation.go`
  - `internal/generator/singbox/model.go`
  - `internal/generator/singbox/generator.go`
  - `internal/generator/singbox/subscription.go`
  - `internal/subscription/render.go`
  - `internal/subscription/index.go`
  - 相关测试文件
- 新增：
  - `internal/configtemplate/templates/snippets/inbound/vmess-http.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vmess-quic.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vmess-httpupgrade.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vless-http.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vless-quic.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vless-httpupgrade.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/inbound/vless-reality-vision.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vmess-http.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vmess-quic.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vmess-httpupgrade.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vless-http.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vless-quic.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vless-httpupgrade.yaml.tmpl`
  - `internal/configtemplate/templates/snippets/outbound/vless-reality-vision.yaml.tmpl`

## 配置与依赖变更

- 未新增依赖。
- 未新增环境变量。
- instance YAML 可新增：
  - `tls.reality`
  - `tls.utls`
  - `flow: xtls-rprx-vision`

## 测试结果

- `go test ./internal/domain ./internal/generator/singbox ./internal/configtemplate ./internal/cli ./internal/subscription`
- `go test ./...`

以上测试均通过。

## 参考资料

- sing-box V2Ray Transport：`https://sing-box.sagernet.org/configuration/shared/v2ray-transport/`
- sing-box TLS / REALITY：`https://sing-box.sagernet.org/configuration/shared/tls/`
- sing-box VLESS outbound：`https://sing-box.sagernet.org/configuration/outbound/vless/`
- Xray XHTTP：`https://xtls.github.io/en/config/transports/xhttp.html`

## 风险与后续建议

- 本次未放行 `xhttp`，避免 vanilla sing-box 生成不兼容配置。
- REALITY 示例中的 key 都是占位符，生产使用前必须替换为实际生成值。
- 如后续运行时切换到明确支持 `xhttp` 的 fork，可单独增加 transport 模型和兼容性测试。
