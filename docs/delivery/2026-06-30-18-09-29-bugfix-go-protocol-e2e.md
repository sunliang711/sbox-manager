# sboxctl 协议端到端测试与修复

## 背景

- 测试主机：`root@10.2.100.157`
- 测试目录：`/tmp/sbox-protocol-e2e`
- sboxctl 版本：`protocol-e2e-test`
- sing-box 版本：`1.13.14`
- 外网连通目标：`https://www.gstatic.com/generate_204`
- 代理下载：使用 `http://user:nopass@10.2.1.107:9605`

## 覆盖范围

- inbound：`http`、`socks5`、`shadowsocks`、`vmess raw`、`vmess websocket`、`vmess grpc`、`vless websocket`、`vless grpc`、`anytls`
- outbound：`direct`、`block`、`ref`、`http`、`socks5`、`shadowsocks`、`vmess raw`、`vmess websocket`、`vmess grpc`、`vless websocket`、`vless grpc`、`anytls`、`trojan`、`hysteria2`

## 修复内容

- 移除 sing-box runtime JSON 中不被 `1.13.14` 接受的 inbound `udp` 字段。
- 移除 Shadowsocks inbound 用户对象中的 `method` 字段，改为只输出 inbound 顶层 `method`。
- WebSocket transport 不再输出小写 `host` 字段，保留 `headers.Host`；`http`/`httpupgrade` 按 schema 保留 host。
- TLS inbound 支持并校验 `certificate_path` 和 `key_path`，避免 `validate` 通过但运行时缺证书失败。
- VLESS `xtls-rprx-vision` flow 禁止和 V2Ray transport 混用，并从 ws/grpc 示例模板中移除该组合。
- 订阅导出时清理服务端证书路径，避免向客户端暴露服务器本地文件路径。

## 验证结果

远端协议矩阵全部通过，除 `block` 按预期阻断外，其余场景均返回 `204`：

| case | expected | result |
| --- | --- | --- |
| outbound-direct | success | PASS |
| outbound-block | blocked | PASS |
| outbound-ref | success | PASS |
| outbound-http | success | PASS |
| outbound-socks5 | success | PASS |
| outbound-shadowsocks | success | PASS |
| outbound-vmess-raw | success | PASS |
| outbound-vmess-ws | success | PASS |
| outbound-vmess-grpc | success | PASS |
| outbound-vless-ws | success | PASS |
| outbound-vless-grpc | success | PASS |
| outbound-anytls | success | PASS |
| outbound-trojan | success | PASS |
| outbound-hysteria2 | success | PASS |

本地验证：

```bash
go test ./...
```

结果：全部通过。

## 风险与说明

- Shadowsocks 2022 方法仍要求用户提供合法 PSK；本次端到端测试使用 `aes-128-gcm` 验证协议链路。
- `trojan` 和 `hysteria2` 仅为 outbound 支持，测试中使用手写 sing-box 服务端配合 sboxctl 生成的客户端 outbound。
