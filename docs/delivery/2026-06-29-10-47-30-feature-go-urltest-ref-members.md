# urltest ref 成员模板实现

## 任务背景

`sboxctl add --template urltest` 原先只创建空的 urltest group，并默认引用 `direct`，无法表达“基于已有 edge/relay 实例作为测速成员”的真实使用方式。参考 `proxystack-go` 后，本次改为在配置层保留 ref 引用，由生成 sing-box 配置时再解析为实际 socks/http outbound。

## 实现方案

- 新增项目域 outbound 类型 `ref`，通过 `ref: <instance>.<inbound>` 引用已有 instance 的 `socks5/http` inbound。
- `sboxctl add` 增加 `--members a,b` 参数，仅 `urltest` 模板可用；`urltest` 模板必须显式指定 members。
- `urltest` 模板根据 members 自动生成 `type: ref` outbounds，并加入 `auto` urltest group。
- `member add/remove/list` 改为维护 ref 成员语义，新增成员时同步创建 ref outbound，删除成员时同步移除对应 ref outbound。
- sing-box 生成器新增完整实例集合入口，在生成阶段将 `type: ref` 解析为原生 `socks/http` outbound，并对通配监听地址做本机连接地址归一化。
- `sboxctl example instance urltest` 和 `add` 生成文件的注释模板同步展示 `type: ref` 与 urltest group 成员写法。

## 文件变更

- `internal/domain/models.go`
- `internal/domain/validation.go`
- `internal/generator/singbox/generator.go`
- `internal/instance/manager.go`
- `internal/cli/sboxctl.go`
- `internal/cli/sboxctl_t03.go`
- `internal/cli/sboxctl_t04.go`
- `internal/runtime/plan.go`
- `docs/cli-spec.md`
- `docs/data-spec.md`

## 测试结果

- `go test ./internal/domain`
- `go test ./internal/instance`
- `go test ./internal/generator/singbox`
- `go test ./internal/cli ./internal/runtime`
- `go test ./...`

全部通过。

## 风险与后续建议

- `ref` 当前限定为跨 instance 引用 `socks5/http` inbound，不允许自引用，避免把流量路由回当前实例造成循环。
- `urltest` 删除最后一个成员会因 group 为空被现有校验拒绝，符合当前“urltest 必须有成员”的约束。
