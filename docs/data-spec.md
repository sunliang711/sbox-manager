# 配置与数据规格

本文定义开发前必须固定的配置、订阅、备份、traffic DB、服务文件和 timer 契约。所有 schema 都是本项目新格式，不兼容参考项目旧格式。

## 1. Schema 严格度

| Schema | 未知字段策略 | 说明 |
| --- | --- | --- |
| `GlobalConfig`、`Instance`、`SubConfig`、`TrafficConfig` | 默认拒绝未知字段 | 避免拼写错误被静默忽略 |
| `RuntimeManifest` | 拒绝未知字段 | runtime 受管文件和服务映射契约 |
| `SubscriptionInput`、`SubscriptionNode`、`SubscriptionIndex` | 拒绝未知字段 | agent/sub 之间的传输契约 |
| `BundleManifest` | 拒绝未知字段 | 订阅 bundle 安全校验契约 |
| `BackupManifest` | 拒绝未知字段 | agent 配置备份恢复契约 |

错误必须包含文件路径或 schema 名称。所有路径字段必须先清理为规范路径，再校验是否仍位于允许的 base dir 或显式绝对路径范围内。

## 2. Agent 全局配置

默认路径：`<base-dir>/config.yaml`，默认 base dir 为 `/opt/sbox-manager`。

```yaml
version: 1
external_host: proxy.example.com

paths:
  bin: bin
  rules: rules
  instances: instances
  runtime: runtime
  generated: runtime/generated
  publish: publish
  traffic: traffic
  downloads: downloads
  logs: logs

port_ranges:
  inbound: 24000-24999
  local_socks: 17000-17999
  local_http: 18000-18999
  api: 10000-10999

defaults:
  log_level: info
  api:
    enabled: true
    listen: 127.0.0.1:10085
  clash_api:
    enabled: false
    listen: 127.0.0.1:19090
  traffic:
    enabled: true
    timezone: Asia/Shanghai
    retention_days: 180
    daily_min_retention_days: 62
    monthly_retention_months: 36
    timeout_seconds: 30
    timer:
      hourly: "0 * * * *"
      daily: "10 0 * * *"
      monthly: "30 0 1 * *"

security:
  require_auth_for_public_socks_http: true
  allow_noauth_public: false
```

| 字段 | 类型 | 必填 | 默认值 | 校验 |
| --- | --- | --- | --- | --- |
| `version` | int | 是 | `1` | 只能为 `1` |
| `external_host` | string | 否 | 无 | 订阅导出前必须有值；允许域名或公网 IP |
| `paths` | object | 否 | 见下表 | 相对路径按 base dir 解析，绝对路径原样使用 |
| `port_ranges` | object | 否 | 见示例 | 支持 `start-end` 或 `{start,end}` |
| `defaults.log_level` | string | 否 | `info` | `trace`、`debug`、`info`、`warn`、`error` |
| `defaults.api.enabled` | bool | 否 | `true` | bool |
| `defaults.api.listen` | string | 否 | `127.0.0.1:10085` | 默认只允许 loopback |
| `defaults.clash_api.enabled` | bool | 否 | `false` | bool |
| `defaults.clash_api.listen` | string | 否 | `127.0.0.1:19090` | 默认只允许 loopback |
| `defaults.traffic.enabled` | bool | 否 | `true` | bool |
| `defaults.traffic.timezone` | string | 否 | `Asia/Shanghai` | IANA 时区 |
| `defaults.traffic.retention_days` | int | 否 | `180` | hourly 保留天数，必须大于 0 |
| `defaults.traffic.daily_min_retention_days` | int | 否 | `62` | daily 最小保留天数，必须大于 0 |
| `defaults.traffic.monthly_retention_months` | int | 否 | `36` | monthly 保留月数，必须大于 0 |
| `defaults.traffic.timeout_seconds` | int | 否 | `30` | stats 请求超时，必须大于 0 |
| `defaults.traffic.timer.*` | string | 否 | 见示例 | cron 风格配置，生成 systemd/launchd 调度时转换 |
| `security.require_auth_for_public_socks_http` | bool | 否 | `true` | bool |
| `security.allow_noauth_public` | bool | 否 | `false` | 只有显式为 true 才允许公网 noauth |

`paths` 默认值：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `bin` | `bin` | `sing-box` 和本项目受管二进制目录 |
| `rules` | `rules` | 规则集和 geo 资源目录 |
| `instances` | `instances` | instance YAML 配置目录 |
| `runtime` | `runtime` | manifest、锁文件和运行期目录 |
| `generated` | `runtime/generated` | 生成的 sing-box JSON 目录 |
| `publish` | `publish` | 订阅 bundle 和 agent backup 输出目录 |
| `traffic` | `traffic` | traffic SQLite 和采集状态目录 |
| `downloads` | `downloads` | 下载缓存目录 |
| `logs` | `logs` | launchd stdout/stderr 和可选应用日志目录 |

约束：

- `instances`、`runtime`、`generated`、`publish`、`traffic`、`downloads`、`logs` 不能互为同一路径。
- `generated` 必须位于 `runtime` 下，避免 `remove --purge` 删除非受管文件。
- `external_host` 不得包含 URL scheme、path、query 或 fragment。
- 端口范围必须在 `1-65535` 内，且 `start <= end`。

## 3. Instance 配置

默认路径：`<base-dir>/instances/<name>.yaml`。文件名必须与 `name` 一致，扩展名为 `.yaml` 或 `.yml`。

```yaml
name: edge-us
enabled: true
role: edge
labels: [us]

api:
  enabled: true
  listen: 127.0.0.1:10085

inbounds:
  - name: vmess-main
    type: vmess
    listen: 0.0.0.0
    port: 24100
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
    subscription:
      enabled: true
      user: alice
      remark: US VMess
      region: US

outbounds:
  - name: proxy-a
    type: shadowsocks
    server: server.example.com
    port: 443
    method: 2022-blake3-aes-256-gcm
    password: change-me

groups:
  - name: auto
    type: urltest
    outbounds: [proxy-a]
    url: http://www.gstatic.com/generate_204
    interval: 300

route:
  default: auto
  rules:
    - type: domain_suffix
      values: [google.com]
      outbound: auto

traffic:
  enabled: true
  scopes: [user, inbound, outbound]
```

顶层字段：

| 字段 | 类型 | 必填 | 默认值 | 校验 |
| --- | --- | --- | --- | --- |
| `name` | string | 是 | 无 | 安全 basename，不能为 `ALL` |
| `enabled` | bool | 否 | `true` | disabled instance 不参与默认生命周期 |
| `role` | string | 否 | `edge` | `edge`、`relay`、`urltest` |
| `labels` | string list | 否 | `[]` | 展示和过滤用 |
| `api` | object | 否 | 继承全局默认 | sing-box V2Ray API / stats 监听 |
| `inbounds` | list | 是 | 无 | 至少一个 inbound |
| `outbounds` | list | 否 | `[]` | 出站代理或直连目标 |
| `groups` | list | 否 | `[]` | selector/urltest 出站集合 |
| `route` | object | 是 | 无 | 默认出站和路由规则 |
| `traffic` | object | 否 | 继承全局默认 | traffic 采集范围 |

`api` 字段：

| 字段 | 类型 | 必填 | 默认值 | 校验 |
| --- | --- | --- | --- | --- |
| `enabled` | bool | 否 | 继承全局 | bool |
| `listen` | string | 否 | 继承全局 | `HOST:PORT`，默认必须是 loopback |
| `token` | string | 非 loopback 时必填 | 无 | 非空，日志和 show 输出必须脱敏 |

`Inbound` 字段：

| 字段 | 类型 | 必填 | 默认值 | 校验 |
| --- | --- | --- | --- | --- |
| `name` | string | 是 | 无 | instance 内唯一 |
| `type` | string | 是 | 无 | `vmess`、`vless`、`anytls`、`shadowsocks`、`socks5`、`http` |
| `listen` | string | 否 | `0.0.0.0` | host，不含端口 |
| `port` | int | 是 | 无 | `1-65535` |
| `tag` | string | 否 | `<type>-<name>` | sing-box tag，instance 内唯一 |
| `udp` | bool | 否 | 协议默认 | 适用于支持 UDP 的协议 |
| `auth.type` | string | socks5/http 必填 | `noauth` | `noauth`、`password` |
| `auth.username` | string | password 必填 | 无 | 非空 |
| `auth.password` | string | password 必填 | 无 | 非空 |
| `tls.enabled` | bool | AnyTLS 必填 | `false` | AnyTLS 必须为 `true`，其他支持 TLS 的 inbound 可按需启用 |
| `tls.server_name` | string | 否 | 无 | TLS SNI |
| `tls.insecure` | bool | 否 | `false` | 是否跳过证书校验，生产环境建议 `false` |
| `tls.alpn` | string list | 否 | `[]` | TLS ALPN 列表 |
| `transport` | object | 否 | 无 | VMess/VLESS 的 V2Ray transport 配置 |
| `users` | list | vmess/vless/anytls/shadowsocks 必填 | 无 | 多用户凭据 |
| `subscription` | object | 否 | `{enabled:false}` | 是否导出订阅节点 |

`InboundUser` 字段：

| 字段 | 类型 | 必填 | 适用 |
| --- | --- | --- | --- |
| `name` | string | 是 | vmess、vless、anytls、shadowsocks |
| `uuid` | string | vmess/vless 必填 | vmess、vless |
| `password` | string | shadowsocks/anytls 必填 | shadowsocks、anytls |
| `method` | string | shadowsocks 可选 | shadowsocks，缺省继承 inbound method |
| `flow` | string | 否 | vless，支持 `xtls-rprx-vision` |
| `alter_id` | int | 否 | vmess，`0` 表示 VMess AEAD |
| `remark` | string | 订阅启用时建议 | 订阅展示名 |
| `tag` | string | 否 | 订阅 tag 覆盖 |

`subscription` 字段：

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `enabled` | bool | 否 | `false` | 是否生成订阅 node |
| `user` | string | enabled 时必填 | 无 | 订阅用户 |
| `server` | string | 否 | 全局 `external_host` | 客户端连接地址 |
| `remark` | string | enabled 时必填 | inbound/user 默认名 | 客户端展示名 |
| `region` | string | 否 | 无 | 两位大写字母 |

`Outbound` 字段：

| 字段 | 类型 | 必填 | 适用 |
| --- | --- | --- | --- |
| `name` | string | 是 | 所有类型 |
| `type` | string | 是 | `direct`、`block`、`ref`、`shadowsocks`、`vmess`、`vless`、`anytls`、`trojan`、`hysteria2`、`socks5`、`http` |
| `ref` | string | ref 必填 | `type=ref`，格式 `<instance>.<inbound>`，按已有 instance 名匹配，只能指向 `socks5/http` inbound |
| `server` | string | 远端类型必填 | 远端类型 |
| `port` | int | 远端类型必填 | 远端类型 |
| `uuid` | string | vmess/vless 必填 | vmess、vless |
| `password` | string | shadowsocks/anytls/trojan/hysteria2 可必填 | 对应协议 |
| `method` | string | shadowsocks 必填 | shadowsocks |
| `security` | string | 否 | vmess 加密参数，Surge 输出为 `encrypt-method` |
| `flow` | string | 否 | vless，支持 `xtls-rprx-vision` |
| `alter_id` | int | 否 | vmess，`0` 表示 VMess AEAD |
| `auth.username` | string | socks5/http 可选 | socks5/http |
| `auth.password` | string | socks5/http 可选 | socks5/http |
| `tls.enabled` | bool | AnyTLS 必填 | 支持 TLS 的远端类型；AnyTLS 必须为 `true` |
| `tls.server_name` | string | 否 | TLS 远端类型 |
| `tls.insecure` | bool | 否 | TLS 远端类型 |
| `tls.alpn` | string list | 否 | TLS 远端类型 |
| `network` | string | vmess 可选 | 仅表示 VMess 底层网络，支持 `tcp`、`udp`；V2Ray transport 必须写入 `transport.type` |
| `transport` | object | 否 | VMess/VLESS 的 V2Ray transport 配置 |

`transport` 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `type` | string | 是 | `http`、`ws`、`quic`、`grpc`、`httpupgrade` |
| `host` | string | 否 | HTTPUpgrade 或 WebSocket Host |
| `hosts` | string list | 否 | HTTP transport 的 host 列表 |
| `path` | string | 否 | HTTP/WebSocket/HTTPUpgrade 请求路径 |
| `method` | string | 否 | HTTP transport 请求方法 |
| `headers` | object | 否 | HTTP/WebSocket/HTTPUpgrade 附加 header |
| `idle_timeout` | duration string | 否 | HTTP/gRPC idle timeout |
| `ping_timeout` | duration string | 否 | HTTP/gRPC ping timeout |
| `max_early_data` | int | 否 | WebSocket early data 大小 |
| `early_data_header_name` | string | 否 | WebSocket early data header |
| `service_name` | string | 否 | gRPC service name |
| `permit_without_stream` | bool | 否 | gRPC keepalive 选项 |

`Group` 字段：

| 字段 | 类型 | 必填 | 默认值 |
| --- | --- | --- | --- |
| `name` | string | 是 | 无 |
| `type` | string | 是 | `selector`、`urltest` |
| `outbounds` | string list | 是 | 无 |
| `url` | string | urltest 必填 | `http://www.gstatic.com/generate_204` |
| `interval` | int | urltest 必填 | `300` |
| `tolerance` | int | 否 | 由 sing-box 默认处理 |

`route` 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `default` | string | 是 | 默认 outbound 或 group 名 |
| `rules` | list | 否 | 结构化路由规则 |

`RouteRule` 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `type` | string | 是 | `domain`、`domain_suffix`、`domain_keyword`、`ip_cidr`、`geoip`、`geosite` |
| `values` | string list | 是 | 匹配值 |
| `outbound` | string | 是 | 已定义 outbound 或 group |

校验规则：

- instance 名、inbound/outbound/group/tag 名称必须稳定、可用于文件名或服务名。
- `ALL` 是 traffic 特殊值，不能作为 instance 名。
- 同一 instance 内 inbound、outbound、group、route 目标引用必须存在。
- 公开监听的 socks5/http inbound 默认必须启用密码鉴权，除非全局显式 `allow_noauth_public=true`。
- API 非 loopback 监听必须配置 token。
- 端口不能在所有 enabled instance 中冲突；`validate --skip-system-ports` 只跳过系统占用探测，不跳过配置内冲突。

## 4. Runtime Manifest

默认路径：`<base-dir>/runtime/manifest.json`。manifest 只记录本项目受管生成物，不进入 agent backup。

```json
{
  "manifest_schema": "sbox.runtime-manifest",
  "manifest_version": 1,
  "config_sha256": "...",
  "instance_sha256": {
    "edge-us": "..."
  },
  "generated_at": "2026-06-28T12:00:00+08:00",
  "files": [
    {
      "instance": "edge-us",
      "path": "/opt/sbox-manager/runtime/generated/sing-box/edge-us.json",
      "relative_path": "generated/sing-box/edge-us.json",
      "sha256": "...",
      "service": "sbox@edge-us.service"
    }
  ]
}
```

约束：

- `relative_path` 使用 slash 分隔，不允许绝对路径、`..` 或反斜杠。
- `path` 必须位于 `paths.generated` 目录下。
- `files` 按 `relative_path` 稳定排序。
- no-change 时不得改写 generated 文件或 manifest，`generated_at` 保留旧值。
- `delete` 只删除 manifest 中受管且落在当前 target scope 内的 generated 文件。
- manifest 写入使用同目录临时文件、fsync、rename，权限为 `0640`。

## 5. 订阅服务配置

默认路径：`<sub-base-dir>/config.yaml`，默认 base dir 为 `/opt/sbox-sub`。

```yaml
version: 1
listen: 127.0.0.1:3003
access:
  type: token
  token: change-me
templates_dir: templates
watch_interval: 2s
watch_debounce: 300ms
managed_config:
  enabled: true
  public_base_url: https://sub.example.com
  interval: 86400
  strict: true
```

| 字段 | 类型 | 必填 | 默认值 | 校验 |
| --- | --- | --- | --- | --- |
| `version` | int | 是 | `1` | 只能为 `1` |
| `listen` | string | 否 | `127.0.0.1:3003` | `HOST:PORT` |
| `access.type` | string | 否 | `none` | `none`、`token` |
| `access.token` | string | token 模式必填 | 无 | 非空 |
| `templates_dir` | path | 否 | `templates` | 相对 sub base dir 或绝对路径 |
| `watch_interval` | duration | 否 | `2s` | 大于 0 |
| `watch_debounce` | duration | 否 | `300ms` | 大于 0 |
| `managed_config.enabled` | bool | 否 | `true` | bool |
| `managed_config.public_base_url` | string | 否 | 无 | 仅允许 http/https，不允许 query/fragment |
| `managed_config.interval` | int | 否 | `86400` | 大于 0 |
| `managed_config.strict` | bool | 否 | `true` | bool |

`access.type=none` 不校验 token，仅允许本地或受控网络部署；公网监听且未配置 token 时 doctor 必须给出 ISSUE。

## 6. 订阅 HTTP 与模板

HTTP 路由：

| 方法 | 路径 | 输出 |
| --- | --- | --- |
| GET | `/health` | JSON 健康状态 |
| GET | `/clash/:user` | Clash 订阅 |
| GET | `/clash/:token/:user` | Clash 订阅 |
| GET | `/premium-clash/:user` | Premium Clash 订阅 |
| GET | `/premium-clash/:token/:user` | Premium Clash 订阅 |
| GET | `/surge/:user` | Surge 订阅 |
| GET | `/surge/:token/:user` | Surge 订阅 |
| GET | `/sing-box/:user` | sing-box 订阅 |
| GET | `/sing-box/:token/:user` | sing-box 订阅 |

鉴权：

- `access.type=none`：推荐使用不带 token 的路径；带 token 的两段路径也可解析，但 token 不参与鉴权且不得写入 access log。
- `access.type=token`：优先读取 path token，也允许 `?token=` query token；缺失返回 401，不匹配返回 403。
- token 只来自 `sboxsub config.yaml`，不得来自 bundle、input 或 URL 生成逻辑之外的隐式来源。

响应：

- 订阅成功响应 `Content-Type: text/plain; charset=utf-8`。
- `/health` 成功返回 `status`、`index`、`users`，reload 失败时追加 `last_error` 且继续服务旧 index。
- 用户不存在或无节点返回 404。
- index 不可用返回 503，code 为 `index_unavailable`。
- 模板解析或渲染失败返回 503，code 为 `template_error`，不得输出半截订阅。
- 错误响应统一为 `{"error":{"code":"...","message":"..."}}`。

模板：

| 模板 | 默认文件名 | 说明 |
| --- | --- | --- |
| Clash | `clash.yaml.tmpl` | Go 标准模板 |
| Premium Clash | `premium-clash.yaml.tmpl` | Go 标准模板 |
| Surge | `surge.conf.tmpl` | Go 标准模板 |
| sing-box | `sing-box.json.tmpl` | Go 标准模板 |

模板查找顺序：

1. `<templates_dir>/sub/<name>`。
2. `<templates_dir>/<name>`。
3. `<sub-base-dir>/templates/sub/<name>`。
4. 内置模板。

模板上下文使用导出字段命名，至少包含：

- `User`。
- `GeneratedAt`。
- `Sources`。
- `Nodes`。
- `ProxyNames`。
- `ClashProxies`。
- `ClashProxyGroups`。
- `ClashRules`。
- `SurgeProxyLines`。
- `SurgeProxyNames`。
- `SurgeRegionGroups`。
- `SurgeRules`。
- `ManagedConfigURL`。
- `ManagedConfigInterval`。
- `ManagedConfigStrict`。
- `TestURL`。

模板执行必须启用缺失 key 报错。自定义模板文件名必须是安全 basename，不允许路径分隔符。

Surge Managed Config：

- `managed_config.enabled=true` 时，Surge 输出第一行 `#!MANAGED-CONFIG <url> interval=<seconds> strict=<true|false>`。
- `public_base_url` 为空时使用当前请求 URL。
- `public_base_url` 非空且 token 模式时使用 `/surge/:token/:user`。
- `public_base_url` 非空且 none 模式时使用 `/surge/:user`。
- Surge 输出仅保留 Surge 支持的协议；当前会输出 `vmess`、`anytls`、`shadowsocks`、`socks5`、`http`，并跳过 `vless` 等不支持协议。VMess WebSocket 会输出 `ws`、`ws-path`、`ws-headers`，`alter_id=0` 时输出 `vmess-aead=true`。

Watcher：

- 监听 `<sub-base-dir>/inputs` 下 `.yaml`、`.yml`、`.json` 普通文件。
- 忽略临时文件、草稿文件、非 input 扩展名和单纯属性变化。
- reload 必须完整加载、校验、构建 index 成功后才替换当前 index。
- reload 失败保留旧 index，记录 `last_error`，日志记录错误类型和 input 目录。

## 7. 订阅 input

文件扩展名：`.yaml`、`.yml`、`.json`。文件名必须是安全 basename，不允许路径分隔符、绝对路径、`..` 或反斜杠。

```yaml
input_schema: sbox.subscription-input
input_version: 1
source: edge-us
generated_at: "2026-06-28T12:00:00+08:00"
external_host: proxy.example.com
nodes:
  - id: edge-us:alice:vmess-main
    user: alice
    protocol: vmess
    server: proxy.example.com
    port: 24100
    tag: edge-us-vmess-main
    remark: US VMess
    region: US
    uuid: 11111111-1111-4111-8111-111111111111
    network: tcp
```

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `input_schema` | string | 是 | 固定 `sbox.subscription-input` |
| `input_version` | int | 是 | 固定 `1` |
| `source` | string | 是 | instance 名或手工来源 |
| `generated_at` | string | 是 | RFC3339 时间 |
| `external_host` | string | 否 | 文件级 server 默认值 |
| `nodes` | list | 是 | 订阅节点 |

`SubscriptionNode` 字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | 全局唯一 |
| `user` | string | 是 | HTTP 路由过滤用户 |
| `protocol` | string | 是 | `vmess`、`vless`、`anytls`、`shadowsocks`、`socks5`、`http`、`sing-box` |
| `server` | string | 条件必填 | 为空时使用文件级 `external_host` |
| `port` | int | 是 | `1-65535` |
| `tag` | string | 是 | 唯一 tag |
| `remark` | string | 是 | 客户端展示名 |
| `region` | string | 否 | 两位大写字母 |
| `uuid` | string | vmess/vless 必填 | UUID |
| `network` | string | 否 | VMess 底层网络，支持 `tcp`、`udp`；V2Ray transport 必须写入 `transport.type` |
| `security` | string | 否 | VMess 加密参数 |
| `flow` | string | 否 | VLESS flow |
| `alter_id` | int | 否 | VMess alterId，`0` 表示 AEAD |
| `method` | string | shadowsocks 必填 | 加密方法 |
| `password` | string | shadowsocks/anytls 必填 | 密码 |
| `auth.type` | string | socks5/http 必填 | `noauth`、`password` |
| `auth.username` | string | password 必填 | 用户名 |
| `auth.password` | string | password 必填 | 密码 |
| `tls.enabled` | bool | AnyTLS 必填 | AnyTLS 必须为 `true`，其他协议按需启用 |
| `tls.server_name` | string | 否 | TLS SNI |
| `tls.insecure` | bool | 否 | 是否跳过证书校验 |
| `tls.alpn` | string list | 否 | TLS ALPN 列表 |
| `transport` | object | 否 | VMess/VLESS 的 V2Ray transport 配置 |
| `udp` | bool | 否 | 显式配置时输出到支持该字段的客户端 |
| `native` | object | sing-box 必填 | sing-box native node 原始片段 |

合并规则：

- 文件按文件名稳定排序。
- `nodes[].id` 全局唯一。
- 同一 `user` 下订阅展示名不能重复。
- 不同 `user` 可以使用相同展示名。
- `server` 为空且文件级 `external_host` 为空时校验失败。
- `sboxsub input show` 默认脱敏 `uuid`、`password`、`auth.password`，`--show-secrets` 才输出明文。

## 8. 订阅 bundle

`sboxctl sub export` 输出 zip。zip 只允许以下成员：

```text
manifest.json
inputs/*.yaml
inputs/*.yml
inputs/*.json
```

`manifest.json`：

```json
{
  "bundle_schema": "sbox.sub-bundle",
  "bundle_version": 1,
  "source": "all",
  "generated_at": "2026-06-28T12:00:00+08:00",
  "inputs_sha256": {
    "edge-us.yaml": "..."
  },
  "template_version": "builtin-v1",
  "access": {"type": "none"}
}
```

约束：

- `inputs_sha256` 的 key 必须与 zip 中 `inputs/` 文件集合完全一致。
- hash mismatch 时不得写入任何 input。
- `access` 当前固定为 `none`，导入时不得用 bundle 配置 token。
- `sboxsub import --replace-all` 只有在 zip、manifest、hash、schema、路径和合并校验全部通过后，才替换旧 managed inputs。

## 9. Agent 配置备份

顶层 `sboxctl export/import` 只处理 agent 配置备份，不接收订阅 bundle。

zip 成员：

```text
backup_manifest.json
config.yaml
instances/*.yaml
```

`backup_manifest.json`：

```json
{
  "backup_schema": "sbox.agent-backup",
  "backup_version": 1,
  "generated_at": "2026-06-28T12:00:00+08:00",
  "files_sha256": {
    "config.yaml": "...",
    "instances/edge-us.yaml": "..."
  }
}
```

不包含：

- runtime manifest。
- generated runtime。
- traffic SQLite。
- downloads。
- logs。
- systemd unit 或 launchd plist。
- sboxsub inputs。

恢复前必须完整校验 `backup_manifest.json`、hash、schema、路径安全和目标覆盖策略。

## 10. Traffic DB

SQLite 使用 GORM 管理模型和迁移，Repository 层隔离 GORM 细节，领域层不得直接依赖 ORM。

连接约定：

- 启用 WAL：`PRAGMA journal_mode=WAL`。
- 设置 busy timeout：默认 `5000ms`。
- `AutoMigrate` 只允许创建表、补字段、补索引；破坏性迁移必须通过显式 schema version 处理。
- `traffic_metadata` 保存 `schema_version`、`created_at`、`updated_at`。
- 迁移失败不得删除或截断既有数据。

表：

- `traffic_records`
- `traffic_baselines`
- `traffic_metadata`

`traffic_records` 字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | uint | GORM 主键 |
| `instance` | string | instance 名 |
| `server` | string | stats API 地址或实例 endpoint 摘要 |
| `period` | string | `hourly`、`daily`、`monthly` |
| `start_ts` | int64 | 周期开始 Unix 秒 |
| `end_ts` | int64 | 周期结束 Unix 秒 |
| `start_time` | string | 按统计时区格式化 |
| `end_time` | string | 按统计时区格式化 |
| `scope` | string | `user`、`inbound`、`outbound` |
| `name` | string | 用户、inbound tag 或 outbound tag |
| `direction` | string | `up`、`down` |
| `bytes` | uint64 | 字节数 |
| `reset_detected` | bool | 是否发现累计计数回退 |
| `created_at` | time | 写入时间 |

唯一键：`instance,period,start_ts,scope,name,direction`。常用索引：`period,start_ts`、`instance,period,start_ts`、`scope,name`。

`traffic_baselines` 字段：

- `instance`
- `scope`
- `name`
- `direction`
- `value`
- `updated_at`

唯一键：`instance,scope,name,direction`。

CSV 导出字段固定为：

```text
instance,server,period,start_time,end_time,scope,name,direction,bytes,created_at
```

保留期：

- hourly 使用 `retention_days`，默认 180 天。
- daily 使用 `max(retention_days, daily_min_retention_days)`，默认至少 180 天且不得低于 62 天。
- monthly 使用 `monthly_retention_months`，默认 36 个月。
- yearly 从 monthly 动态聚合，不落库、不单独清理。
- CLI `--retention-days` 只覆盖本次命令中的 hourly/daily 计算；`--monthly-retention-months` 只覆盖本次命令中的 monthly 清理计算。

## 11. 服务文件与 timer

默认 Linux 运行用户和组为 `sbox:sbox`。目录默认权限为 `0750`，普通配置、manifest、SQLite 默认权限为 `0640`，systemd unit 和 launchd plist 默认权限为 `0644`。

systemd instance service：

- unit：`sbox@<instance>.service`
- `User=sbox`
- `Group=sbox`
- `ExecStart=<base-dir>/bin/sing-box run -c <generated-json>`
- `WorkingDirectory=<base-dir>`
- `Restart=on-failure`
- `NoNewPrivileges=true`
- `ProtectSystem=strict`
- `ProtectHome=true`
- `PrivateTmp=true`
- `ReadWritePaths=<base-dir>/runtime <base-dir>/traffic <base-dir>/logs`
- `SyslogIdentifier=sbox-<instance>`

launchd instance plist：

- label：`com.sbox-manager.<instance>`
- `ProgramArguments` 对应 sing-box run 命令。
- `WorkingDirectory=<base-dir>`。
- `RunAtLoad=false`。
- `StandardOutPath=<base-dir>/logs/<label>.out.log`。
- `StandardErrorPath=<base-dir>/logs/<label>.err.log`。
- 不写 `KeepAlive`，避免安装后隐式自启。

systemd subscription service：

- unit：`sboxsub.service`
- `User=sbox`
- `Group=sbox`
- `ExecStart=<sboxsub-bin> --base-dir <sub-base-dir> serve`
- `WorkingDirectory=<sub-base-dir>`
- `Restart=on-failure`
- `NoNewPrivileges=true`
- `ProtectSystem=strict`
- `ProtectHome=true`
- `PrivateTmp=true`
- `ReadWritePaths=<sub-base-dir>`
- `SyslogIdentifier=sboxsub`

launchd subscription plist：

- label：`com.sbox-manager.sboxsub`
- `ProgramArguments` 对应 `sboxsub --base-dir <sub-base-dir> serve`。
- `WorkingDirectory=<sub-base-dir>`。
- `RunAtLoad=false`。
- `StandardOutPath=<sub-base-dir>/logs/sboxsub.out.log`。
- `StandardErrorPath=<sub-base-dir>/logs/sboxsub.err.log`。
- 不写 `KeepAlive`，避免安装后隐式自启。

traffic timer：

| 周期 | systemd service | systemd timer | systemd `OnCalendar` | launchd label | launchd `StartCalendarInterval` | 执行动作 |
| --- | --- | --- | --- | --- | --- | --- |
| hourly | `sbox-traffic-hourly.service` | `sbox-traffic-hourly.timer` | `*-*-* *:00:00` | `com.sbox-manager.traffic.hourly` | `{Minute=0}` | `sboxctl traffic collect hourly --instance ALL` |
| daily | `sbox-traffic-daily.service` | `sbox-traffic-daily.timer` | `*-*-* 00:10:00` | `com.sbox-manager.traffic.daily` | `{Hour=0,Minute=10}` | `sboxctl traffic collect daily --instance ALL` |
| monthly | `sbox-traffic-monthly.service` | `sbox-traffic-monthly.timer` | `*-*-01 00:30:00` | `com.sbox-manager.traffic.monthly` | `{Day=1,Hour=0,Minute=30}` | `sboxctl traffic collect monthly --instance ALL` |

traffic systemd service：

- `Type=oneshot`
- `User=sbox`
- `Group=sbox`
- `ExecStart=<sboxctl-bin> --base-dir <base-dir> traffic collect <period> --instance ALL`
- `WorkingDirectory=<base-dir>`
- `NoNewPrivileges=true`
- `ProtectSystem=strict`
- `ProtectHome=true`
- `PrivateTmp=true`
- `ReadWritePaths=<base-dir>/traffic <base-dir>/logs`

traffic systemd timer：

- `Persistent=true`
- `AccuracySec=1min`
- `[Install] WantedBy=timers.target`

traffic launchd plist：

- `ProgramArguments` 对应 `sboxctl --base-dir <base-dir> traffic collect <period> --instance ALL`。
- `WorkingDirectory=<base-dir>`。
- 使用 `StartCalendarInterval` 表达 hourly、daily、monthly 调度。
- `RunAtLoad=false`。
- `StandardOutPath=<base-dir>/logs/<label>.out.log`。
- `StandardErrorPath=<base-dir>/logs/<label>.err.log`。

服务管理约束：

- `service install` 写入或更新 systemd unit 后必须执行 `systemctl daemon-reload`。
- `traffic timer install` 写 service/timer 或 plist 后自动启用 timer；systemd 使用 `enable --now`，launchd 使用 `bootstrap` 后按需 `enable`。
- `traffic timer enable` 可用于重复确保自动调度已启用。
- `traffic timer uninstall` 删除前先停用并卸载受管 timer/service 或 plist，不删除 traffic DB。
- 所有 systemctl、journalctl、launchctl、log 调用必须使用参数数组，不得拼接 shell 字符串。

## 12. 外部规格依据

- sing-box 配置以官方文档为准：`https://sing-box.sagernet.org/configuration/`。
- sing-box V2Ray API/stats 以官方 V2Ray API 文档为准：`https://sing-box.sagernet.org/configuration/experimental/v2ray-api/`。
- systemd unit/timer 字段以 freedesktop.org systemd 手册为准。
- launchd plist 字段以 macOS `launchd.plist(5)` 手册为准。
