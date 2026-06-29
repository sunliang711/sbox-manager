# sboxctl example 协议示例与实例模板补全

## 任务背景

`sboxctl example` 的配置示例没有覆盖当前项目已经支持的全部 inbound/outbound 协议；同时 `sboxctl add` 生成的实例配置虽然带基础说明，但缺少可直接参考的全协议注释模板。

## 实现方案

- 补全 `sboxctl example inbound`，覆盖 `vmess`、`vless`、`anytls`、`shadowsocks`、`socks5`、`http`。
- 补全 `sboxctl example outbound`，覆盖 `direct`、`block`、`shadowsocks`、`vmess`、`vless`、`anytls`、`trojan`、`hysteria2`、`socks5`、`http`。
- 在 `sboxctl add` 生成的 instance YAML 中追加全协议参考模板，并保持整段注释状态，避免影响默认可运行配置。
- 更新测试，确保 `example` 输出和 `add` 生成文件都不会再漏掉 VLESS、AnyTLS 等协议。

## 文件变更

- `internal/cli/sboxctl_t04.go`
- `internal/cli/command_test.go`
- `internal/instance/manager.go`
- `internal/instance/manager_test.go`

## 测试结果

- `go test ./internal/cli`
- `go test ./internal/instance`
- `go test ./...`

三组测试均通过。

## 风险与后续建议

- `add` 生成的协议模板为注释参考，不参与严格配置加载；用户启用时仍需自行调整端口、凭据和路由引用。
