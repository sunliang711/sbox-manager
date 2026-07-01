# 其它协议模板检查与修复

## 背景

在修复 VMess WebSocket 示例后，继续检查其它 inbound/outbound/transport 协议模板，重点核对 WebSocket、HTTPUpgrade、QUIC 这类容易受 transport/TLS 字段组合影响的协议。

## 发现与处理

- HTTPUpgrade outbound 和通用 transport 模板中存在活动 `method: GET` 字段，但 sing-box HTTPUpgrade transport 不接受 `method`，已改为注释说明。
- VLESS WebSocket 和 VMess/VLESS HTTPUpgrade 模板默认暴露 `alpn: [h2, http/1.1]`，对 HTTP/1.1 upgrade 类传输有误导风险，已改为注释，服务端明确需要时再启用。
- VMess QUIC inbound 模板缺少 QUIC transport 必需的 TLS 示例，已补齐 `tls.enabled`、`alpn: [h3]`、证书和私钥路径示例。

## 验证

- `go test ./internal/cli ./internal/configtemplate ./internal/generator/singbox ./internal/subscription`
- `go test ./...`
- `sboxctl example` 针对 WebSocket、HTTPUpgrade、QUIC 的输出扫描：
  - WebSocket 示例无活动 `alpn/host/hosts/method/idle_timeout/ping_timeout/max_early_data/early_data_header_name`。
  - HTTPUpgrade 示例无活动 `alpn/method`。
  - VMess QUIC inbound 示例包含 TLS 和 `alpn: [h3]`。

## 备注

本次尝试连接 `root@10.2.13.230` 做远程 sing-box check 时 SSH 超时，因此本轮验证以本地模板输出扫描和 Go 测试为准。
