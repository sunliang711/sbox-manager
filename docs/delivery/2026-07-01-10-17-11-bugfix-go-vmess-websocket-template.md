# VMess WebSocket 模板修复记录

## 问题背景

- `sboxctl example outbound vmess` 输出的 VMess WebSocket 示例包含多个默认启用字段。
- 其中 `host`、`hosts`、`method`、`idle_timeout`、`ping_timeout` 不属于 sing-box WebSocket transport 的正确字段集合。
- `alpn` 和 WebSocket early data 虽然是可选能力，但默认启用会让通用示例不够稳妥。

## 根因分析

- WebSocket 模板复用了部分 HTTP/gRPC/HTTPUpgrade 字段，导致示例需要用户手工注释后才接近可用配置。
- 通用 `transport.ws` 片段和 VMess/VLESS WebSocket 片段存在同类字段问题。

## 修复方案

- 移除 VMess/VLESS WebSocket 模板中的活动 `host` 字段，统一通过 `headers.Host` 表达 WebSocket Host。
- 移除 VMess WebSocket outbound 中活动的 `hosts`、`method`、`idle_timeout`、`ping_timeout`。
- 将 VMess WebSocket outbound 的 `alpn`、`max_early_data`、`early_data_header_name` 改为注释示例。
- 将通用 `transport.ws` 的 early data 字段改为注释示例。
- 补充 CLI 示例测试，确保模板不再输出活动的错误字段。

## 验证结果

- `go test ./internal/cli ./internal/configtemplate ./internal/generator/singbox ./internal/subscription` 通过。
- `sboxctl example outbound vmess` 输出中，活动 WebSocket transport 字段为 `type`、`path`、`headers.Host`。

## 风险与后续建议

- 本次只调整示例模板，不改变已加载配置模型和 runtime 生成逻辑。
- 已有配置文件如果手工保留了这些字段，runtime 生成阶段仍会按当前生成器规则处理。
