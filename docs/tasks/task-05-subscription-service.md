# T05 订阅服务

## 目标

实现新订阅 schema、bundle、订阅渲染和 `sboxsub` HTTP 服务。

## 范围

- 订阅 input schema。
- index 构建。
- `sboxctl sub export`。
- `sboxctl sub validate-inputs`。
- `sboxsub import`。
- Clash/Premium Clash/Surge/sing-box 渲染。
- `sboxsub` CLI。
- HTTP server。
- watcher。

## 技术方案

`sboxctl sub export` 负责从 instance 生成订阅 bundle。`sboxsub import` 负责导入 bundle，`sboxsub` 运行时只读取自己的配置和 inputs。

订阅 input、bundle manifest、hash、zip 成员、token 存储和 `access.type=none` 限制以 `docs/data-spec.md` 为准。

HTTP 路由：

- `/health`
- `/sub/:user`
- `/sub/:token/:user`
- `/premium_sub/:user`
- `/premium_sub/:token/:user`
- `/surge_sub/:user`
- `/surge_sub/:token/:user`
- `/sing-box/:user`
- `/sing-box/:token/:user`

## 验收标准

- input 非法时 serve 启动失败。
- reload 失败保留旧 index。
- token 缺失返回 401。
- token 错误返回 403。
- `access.type=none` 支持无 token 路径；token 模式支持 path token 和 query token。
- 用户不存在返回 404。
- 模板错误返回 503。
- bundle 导入先完整校验再写入。
- bundle zip 只允许 `manifest.json` 和 `inputs/*` 安全成员。
- `access.token` 只来自 sboxsub config，不来自 bundle。
- `sboxctl sub export --dry-run` 和 `--summary` 不写 publish。
- 日志不包含 token/password/完整订阅。

## 风险

- 模板能力过强会带来兼容负担。本项目固定使用 Go 标准模板，不引入其他模板引擎。
