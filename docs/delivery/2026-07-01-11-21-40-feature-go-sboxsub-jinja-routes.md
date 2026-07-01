# sboxsub Jinja 订阅路径对齐交付说明

## 任务背景

- 将 `sboxsub` 订阅路径调整为与 `../proxystack-go` 一致。
- 订阅返回内容改为使用 Jinja 模板，并参考 `../proxystack-go/templates/sub`。
- `sboxsub serve` 增加启动和 HTTP 请求日志。

## 实现方案

- HTTP 路由调整为 `/sub/:user`、`/sub/:token/:user`、`/premium_sub/:user`、`/premium_sub/:token/:user`、`/surge_sub/:user`、`/surge_sub/:token/:user`，并保留本项目额外的 `/sing-box/:user`、`/sing-box/:token/:user`。
- 使用 `github.com/flosch/pongo2/v6` 渲染 Jinja 模板，内置模板通过 `templates/sub/*.j2` embed。
- 复制 `proxystack-go` 的 Clash、Premium Clash、Surge Jinja 模板，并新增 sing-box Jinja 模板。
- 添加启动日志和脱敏 HTTP 请求日志，日志不输出 token、query、密码或订阅正文。

## 文件变更

- 修改 `internal/subserver/server.go`：路由解析、启动日志、请求日志。
- 修改 `internal/subscription/render.go`：Jinja 渲染、模板上下文、过滤器、变量校验。
- 新增 `templates/embed.go` 与 `templates/sub/*.j2`。
- 修改订阅服务测试与相关文档。
- 新增 `pongo2` 依赖。

## 配置与依赖变更

- 新增 Go 依赖：`github.com/flosch/pongo2/v6 v6.1.0`。
- 无新增配置项。
- 无数据库或外部服务变更。

## 测试结果

- `go test ./...` 通过。

## 风险与后续建议

- 内置 Clash、Premium Clash、Surge 模板内容较 `sbox-manager` 原先模板明显更丰富，返回体会更接近 `proxystack-go`，但也会包含其默认规则集引用。
- `go mod tidy` 曾因 `proxy.golang.org` 下载 `github.com/kr/pretty` 超时未完成；已手动整理直接依赖，并通过全量测试验证。
