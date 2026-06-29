# sboxctl example 示例补全交付记录

## 背景

`sboxctl example` 已声明支持 `[TYPE]` 参数，但旧实现只读取第一段 kind，导致 `sboxctl example inbound vmess`、`sboxctl example outbound shadowsocks` 等命令不能按协议输出示例。旧示例也缺少完整字段和注释，对 Shadowsocks 2022、VMess、不同 outbound 协议的配置项说明不足。

## 变更

- `example` 命令现在会读取第二个参数 `TYPE`，并按 `global`、`instance`、`inbound`、`outbound`、`group`、`route`、`traffic` 分发示例。
- `inbound` 支持 `vmess`、`shadowsocks`、`shadowsocks22`、`socks5`、`http`、`all` 示例。
- `outbound` 支持 `direct`、`block`、`shadowsocks`、`shadowsocks22`、`vmess`、`trojan`、`hysteria2`、`socks5`、`http`、`all` 示例。
- `instance` 支持 `edge`、`relay`、`urltest` 模板示例。
- `group` 支持 `selector`、`urltest`、`all` 示例。
- `route` 支持所有当前模型支持的规则类型示例。
- 示例中加入字段级注释，并保持输出为可复制的 YAML 片段。
- 未知 TYPE 会返回支持列表，便于用户修正命令。

## 验证

- 本地：`go test ./...` 通过。
- 本地：`go run ./cmd/sboxctl example inbound shadowsocks22` 输出 Shadowsocks 2022 字段和注释。
- 本地：`go run ./cmd/sboxctl example outbound` 输出全部 outbound 协议示例。
- 本地：将 `go run ./cmd/sboxctl example instance edge` 输出写入实例配置后，`sboxctl validate` 通过。
- 远端：交叉编译 `linux/amd64` 后上传到 `root@10.2.100.157:/opt/sbox-manager-example-20260629003000`。
- 远端：`example` smoke 共 11 项，0 失败，报告路径 `/opt/sbox-manager-example-20260629003000/example-smoke-rerun.log`。
