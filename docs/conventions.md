# sbox-manager 开发规范

## 1. 基本原则

- 所有用户面向文档使用简体中文。
- 日志消息使用英文。
- 功能点参考 `/Users/eagle/Sync/proxy/proxystack-go` 和 `/Users/eagle/.local/apps/init/tools/xray/traffic`，但不实现旧格式兼容。
- 修改代码时优先保持模块边界清晰，不做无关重构。
- 所有外部命令使用参数数组执行，禁止拼接 shell 字符串。

## 2. 项目结构

```text
cmd/
  sboxctl/
  sboxsub/
internal/
  backup/
  config/
  diagnostics/
  domain/
  generator/singbox/
  install/
  runtime/
  service/
  subscription/
  subserver/
  traffic/
  version/
docs/
  tasks/
```

`cmd/*/main.go` 只负责启动命令树，业务逻辑放在 `internal/`。

## 3. Go 编码规范

- 包名使用小写单词，不使用下划线。
- 导出类型和方法必须有注释。
- 每个函数应有清晰职责，避免同时做加载、校验、写入和服务调用。
- 错误使用 `fmt.Errorf("context: %w", err)` 包装。
- 不忽略 error，确需忽略时必须写明原因。
- 共享状态必须用 `sync.Mutex`、`sync.RWMutex`、`atomic.Value` 或 channel 保护。
- goroutine 必须有退出机制。

## 4. 分层约定

- CLI 层：参数绑定、输出、错误摘要。
- Service 层：业务流程编排。
- Domain 层：模型、默认值、校验规则。
- Repository 层：SQLite 访问，使用 GORM 管理模型和迁移。
- Generator 层：纯输入到输出，不做文件写入和服务调用。
- Runtime 层：diff、manifest、原子写入。

## 5. 配置规范

- 用户配置使用 YAML。
- 生成物使用稳定 JSON。
- 配置字段使用 `snake_case`。
- 运行期只从明确目录读取文件。
- 所有路径必须规范化，拒绝路径穿越。
- 安全相关默认值必须保守。

## 6. CLI 副作用规范

命令命名以 [CLI 命令规格](cli-spec.md) 为准。

只读命令不得写文件、启动服务或下载安装：

- `validate`
- `check`
- `render`
- `export-config`
- `doctor`
- `ipinfo`
- `list`
- `traffic show`
- `traffic watch`
- `traffic summarize`
- `traffic list`
- `traffic check`
- `sub validate-inputs`

写 runtime / publish / traffic 数据或报表只允许：

- `start`
- `restart`
- `sub export`
- `traffic collect`
- `traffic export`
- `traffic cleanup`
- `traffic timer run`

写 traffic 配置只允许：

- `traffic edit config`

写备份文件只允许：

- `export`

写配置恢复只允许：

- `import`

服务管理只允许：

- `stop`
- `status`
- `logs`
- `enable`
- `disable`
- `service install`
- `service uninstall`
- `service start`
- `service stop`
- `service restart`
- `service status`
- `service logs`
- `service log`
- `service enable`
- `service disable`
- `traffic timer install`
- `traffic timer uninstall`
- `traffic timer enable`
- `traffic timer disable`
- `traffic timer status`
- `traffic timer logs`

`start/restart` 是特殊生命周期命令，允许先写 runtime，再调用服务管理器；`stop/status/logs/enable/disable` 不得写 runtime。`restart` 是显式强制重启，no-change 时不得改写 generated 或 manifest，但仍调用目标服务的 restart。

服务管理器通过 `--service-manager auto|systemd|launchd` 指定，Linux 下 `auto` 解析为 `systemd`，macOS 下 `auto` 解析为 `launchd`。

## 7. 文件写入规范

- 生成文件必须先写同目录临时文件。
- fsync 后 rename。
- 内容不变时不改写。
- 写入后修复权限。
- manifest 必须原子替换。

默认权限：

- 目录：`0750`
- 配置文件：`0640`
- 二进制：`0750`
- SQLite：`0640`

## 8. GORM 与迁移规范

- Repository 层使用 GORM，Domain 层不得引用 GORM 类型。
- SQLite 连接启用 WAL 和 busy timeout。
- `AutoMigrate` 只用于非破坏性迁移。
- `traffic_metadata` 必须记录 schema version。
- 破坏性 schema 变更必须显式实现迁移步骤，并保证失败不删除既有数据。

## 9. 日志规范

- 使用 Zerolog 结构化日志。
- 日志字段使用英文。
- 禁止记录 token、password、secret、UUID 明文和完整订阅内容。
- HTTP access log 不记录完整订阅 path token。
- 错误日志包含 `err` 字段。

## 10. 安全规范

- API 默认监听 `127.0.0.1`。
- 非 loopback 管理 API 必须配置 token 或 secret。
- 公开 socks/http inbound 默认要求鉴权。
- 内置下载源必须具备可信 checksum 元数据；自定义远端 URL 必须显式提供 sha256。
- 解压归档必须校验成员路径。
- zip/tar 拒绝绝对路径、`..`、反斜杠路径和未知成员。
- 生产默认不启用 debug/pprof。

## 11. 流量统计规范

- 不依赖 sing-box reset。
- 使用 baseline 自算 delta。
- 计数回退时记录 `reset_detected`。
- hourly 只从 API 当前计数生成。
- daily 只从 hourly 聚合。
- monthly 只从 daily 聚合。
- yearly 动态聚合，不落库。
- `show` 覆盖当前周期时追加未落库增量。
- `traffic timer install` 只写调度文件，不启用、不启动。
- `traffic timer run` 等价立即执行对应 `collect ... --instance ALL`，不调用服务管理器。

## 12. 订阅规范

- `sboxsub` 不读取 agent 配置。
- 启动时 input 非法必须失败。
- reload 失败保留旧 index。
- 默认输出不展示敏感字段。
- bundle 导入先完整校验，再原子写入。
- 模板错误返回 503，不能输出半截订阅。
- 订阅 input、bundle manifest 和 hash 规则以 `docs/data-spec.md` 为准。

## 13. 测试规范

- 单元测试与被测文件同目录。
- 生成器必须有 golden 测试。
- CLI 副作用必须用 fake filesystem/service manager 测试。
- traffic 必须用 fixture 覆盖正常 delta、计数回退、ALL 小计、动态当前周期。
- HTTP 服务必须覆盖鉴权、404、503、reload 失败保留旧 index。
