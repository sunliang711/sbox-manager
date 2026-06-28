# T02 配置模型与校验

## 目标

实现新项目配置 schema、领域模型、默认值和校验逻辑。

## 范围

- 全局配置加载。
- instance 配置加载。
- sub 服务配置加载。
- traffic 配置加载。
- 端口范围解析和分配。
- 安全校验。

## 技术方案

使用 YAML 作为用户配置格式。领域模型不兼容旧 `/Users/eagle/Sync/proxy/proxystack-go`，字段直接围绕 sing-box 设计。

详细 schema 严格度、sub config、subscription input、backup manifest 和 traffic DB 契约以 `docs/data-spec.md` 为准。

核心模型：

- `GlobalConfig`
- `Instance`
- `Inbound`
- `Outbound`
- `Group`
- `RouteRule`
- `SubConfig`
- `TrafficConfig`

## 验收标准

- 默认配置可通过校验。
- 非 loopback socks/http 无鉴权默认失败。
- 引用不存在时聚合报错。
- 端口冲突可被检测。
- 不支持的 group/rule 类型 fail fast。
- 订阅、备份和 traffic 相关 schema 拒绝未知字段。

## 风险

- schema 过度贴近 sing-box 原始 JSON 会降低可维护性。需要保留面向用户的抽象层。
