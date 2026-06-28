# CLI 命令规格

本文定义 `sboxctl` 和 `sboxsub` 的命令树。命令命名尽量贴近参考项目 `ps-agent`、`ps-sub` 和 `xray-traffic`，但数据模型、配置格式和运行语义以本项目 sing-box 新模型为准。

## 1. 全局约定

- `sboxctl` 通过全局 `--base-dir DIR` 指定 agent 环境目录，默认 `/opt/sbox-manager`。
- `sboxsub` 通过全局 `--base-dir DIR` 指定订阅服务环境目录，默认 `/opt/sbox-sub`。
- 服务管理器通过全局 `--service-manager auto|systemd|launchd` 指定，默认 `auto`；Linux 解析为 `systemd`，macOS 解析为 `launchd`。
- `sboxsub` 支持全局 `--listen HOST:PORT` 覆盖 HTTP 监听地址，`serve --host` 和 `serve --port` 可进一步覆盖；默认监听地址以 `SubConfig` 为准。
- `sboxctl traffic` 支持局部持久参数 `--db FILE`、`--timezone TZ`、`--retention-days N`、`--timeout SECONDS`，未传时读取 agent 配置默认值；`cleanup records` 额外支持 `--monthly-retention-months N`。
- CLI 日志消息使用英文，面向用户的摘要和错误说明使用简体中文。
- 默认不自动提权，权限不足时失败并输出目标路径、操作和底层错误摘要。

## 2. 副作用分类

| 分类 | 命令 |
| --- | --- |
| 只读 | `version`、`list`、`validate`、`check`、`render *`、`export-config`、`doctor`、`ipinfo`、`traffic show`、`traffic watch`、`traffic summarize`、`traffic list`、`traffic check`、`sub validate-inputs` |
| 写 agent 配置 | `init`、`config`、`add`、`clone`、`member add/remove`、`remove` |
| 写 sub 配置 | `sboxsub init`、`sboxsub config`、`sboxsub input edit/clone/set-host/remove`、`sboxsub clear` |
| 写 runtime/publish/traffic 数据或报表 | `start`、`restart`、`sub export`、`traffic collect`、`traffic export`、`traffic cleanup`、`traffic timer run` |
| 写 traffic 配置 | `traffic edit config` |
| 写备份文件 | `export` |
| 写配置恢复 | `import` |
| 服务管理器 | `start`、`stop`、`restart`、`status`、`logs`、`enable`、`disable`、`service *`、`traffic timer install/uninstall/enable/disable/status/logs` |
| 下载/安装 | `setup`、`install`、`update`、`uninstall` |
| HTTP 运行 | `sboxsub serve` |

`check` 必须只做完整编译和 diff 预览，不写 runtime，不调用服务管理器。`start/restart` 是本项目唯一会同时 apply runtime plan 和调用实例服务管理器的生命周期命令；`stop/status/logs/enable/disable` 只调用服务管理器。`restart` 是显式强制重启：即使 runtime plan 为 no-change，也会在 `sing-box check` 通过后重启目标服务。

## 3. `sboxctl`

### 3.1 初始化与配置

```bash
sboxctl [--base-dir DIR] init [--external-host HOST] [--force]
sboxctl [--base-dir DIR] setup [--external-host HOST] [--force] [--start]
sboxctl [--base-dir DIR] config [INSTANCE] [--editor CMD] [--check-only]
sboxctl example [global|instance|inbound|outbound|group|route|traffic] [TYPE]
```

- `init` 创建标准目录和默认 `config.yaml`，不下载、不安装服务。
- `setup` 依次执行幂等 `init`、`install all`、`service install`，传 `--start` 时继续执行 `start`。
- `config` 使用相邻草稿文件编辑全局配置或 instance 配置，校验失败不得覆盖原文件。
- `example` 输出可复制的 YAML 片段到 stdout，不读取 `--base-dir`。

### 3.2 实例管理

```bash
sboxctl [--base-dir DIR] add NAME [--template edge|relay|urltest] [--from-file FILE] [--allocate-ports|--keep-template-ports] [--edit|--no-edit] [--editor CMD]
sboxctl [--base-dir DIR] list [--verbose] [--check-system-ports]
sboxctl [--base-dir DIR] clone SOURCE TARGET [--allocate-ports] [--edit|--no-edit] [--editor CMD]
sboxctl [--base-dir DIR] member list INSTANCE GROUP
sboxctl [--base-dir DIR] member add INSTANCE GROUP MEMBER
sboxctl [--base-dir DIR] member remove INSTANCE GROUP MEMBER
sboxctl [--base-dir DIR] remove NAME [--purge]
```

- `add`、`clone` 默认自动分配端口并打开编辑器，`--no-edit` 用于脚本化场景。
- `member` 只维护 `selector` 或 `urltest` group 的 outbound 成员，用于贴近参考项目的 auto 成员维护体验。
- `remove` 默认只归档或删除 instance 配置；`--purge` 才清理 manifest 中关联的 generated 文件。

### 3.3 校验与渲染

```bash
sboxctl [--base-dir DIR] validate [TARGET] [--skip-system-ports]
sboxctl [--base-dir DIR] check [TARGET] [--skip-system-ports]
sboxctl [--base-dir DIR] render model [--skip-system-ports]
sboxctl [--base-dir DIR] render sing-box INSTANCE [--skip-system-ports]
sboxctl [--base-dir DIR] render sub [--input-dir DIR] [--skip-system-ports]
sboxctl [--base-dir DIR] export-config clash|premium-clash|surge|sing-box USER
```

- `validate` 聚合输出配置错误。
- `check` 输出 create/update/delete/no-change 和建议重启服务。
- `render` 和 `export-config` 只输出到 stdout，不写 publish 或 runtime。

### 3.4 生命周期与服务文件

```bash
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] start [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] stop [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] restart [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] status [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] logs [TARGET] [--follow|-f]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] enable [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] disable [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] service install [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] service uninstall [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] service start|stop|restart|status|enable|disable [TARGET]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] service logs|log [TARGET] [--follow|-f]
```

- `TARGET` 为空时作用于全部 enabled instance；传 `NAME` 时只作用于该 instance。
- `start/restart` 调服务管理器前必须执行完整生成、`sing-box check` 和 runtime plan apply。
- `start` 用于确保目标服务运行，no-change 时不改写 generated 或 manifest，但仍可调用服务管理器 start。
- `restart` 用于强制重启目标服务，no-change 时不改写 generated 或 manifest，但仍调用服务管理器 restart。
- `service install/uninstall` 只写 systemd unit 或 launchd plist，不启动服务。

### 3.5 安装、订阅、备份与诊断

```bash
sboxctl [--base-dir DIR] install sing-box|rules|all [--version V] [--source SOURCE] [--sha256 HASH] [--archive-member NAME]
sboxctl [--base-dir DIR] update sing-box|rules|all [--version V] [--source SOURCE] [--sha256 HASH] [--archive-member NAME]
sboxctl [--base-dir DIR] uninstall sing-box|rules|all [--purge]
sboxctl version [sing-box|rules]

sboxctl [--base-dir DIR] sub export [INSTANCE] [-o OUTPUT] [--summary|--dry-run]
sboxctl sub validate-inputs --input-dir DIR

sboxctl [--base-dir DIR] export [-o OUTPUT]
sboxctl [--base-dir DIR] import BACKUP [--force]
sboxctl [--base-dir DIR] doctor
sboxctl [--base-dir DIR] ipinfo INSTANCE [--family all|ipv4|ipv6] [--timeout SECONDS]
```

- `sub export` 生成订阅 bundle，缺少 `external_host` 时失败。
- 顶层 `export/import` 只处理新格式 agent 配置备份，不接收订阅 bundle；备份包使用 `backup_manifest.json`，不包含 runtime manifest。
- `install all` 和 `update all` 不包含 self update。
- 内置 source 必须具备可信 checksum 元数据；自定义 `http(s)://` source 必须显式传 `--sha256`，本地文件可传 `--sha256` 做完整性校验。

### 3.6 流量统计

```bash
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] collect hourly --instance NAME|ALL [--at RFC3339]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] collect daily --instance NAME|ALL [--date YYYY-MM-DD]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] collect monthly --instance NAME|ALL [--month YYYY-MM]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] show current --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] show hourly|daily --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--date YYYY-MM-DD|--from YYYY-MM-DD --to YYYY-MM-DD|--days N] [--limit N]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] show monthly --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--month YYYY-MM|--months N] [--limit N]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--retention-days N] [--timeout SECONDS] show yearly --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--year YYYY|--years N] [--limit N]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] [--timeout SECONDS] watch current --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--interval SECONDS] [--count N] [--no-clear]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] summarize hourly|daily|monthly --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--date YYYY-MM-DD|--from YYYY-MM-DD --to YYYY-MM-DD|--days N]
sboxctl [--base-dir DIR] traffic [--db FILE] [--timezone TZ] export hourly|daily|monthly --instance NAME|ALL [--scope user|inbound|outbound] [--name NAME] [--date YYYY-MM-DD|--from YYYY-MM-DD --to YYYY-MM-DD|--days N] [--format csv] [--output FILE]
sboxctl [--base-dir DIR] traffic [--db FILE] list instances
sboxctl [--base-dir DIR] traffic [--db FILE] [--retention-days N] [--monthly-retention-months N] cleanup records [--period hourly|daily|monthly|all] [--dry-run]
sboxctl [--base-dir DIR] traffic [--db FILE] check config|health
sboxctl [--base-dir DIR] traffic edit config [--editor CMD]
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] traffic timer install|uninstall
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] traffic timer enable|disable|status
sboxctl [--base-dir DIR] [--service-manager auto|systemd|launchd] traffic timer logs [--follow|-f]
sboxctl [--base-dir DIR] traffic timer run hourly|daily|monthly
```

流量子命令保留 `xray-traffic` 的动词和名词结构；差异是 `hourly` 使用 sing-box 累计计数和本项目 baseline 自算 delta，不依赖 reset 语义。

- `--instance ALL` 表示全部配置实例，`ALL` 只在 traffic 命令中作为特殊值。
- `show` 查询为空时输出 `No records found` 并返回 0；采集目标不可达时返回非零。
- `summarize` 输出查询范围、instance、scope、name 和汇总表。
- `export` 默认输出 CSV 到 stdout，指定 `--output` 时写文件。
- `cleanup records --dry-run` 只输出将删除的记录数量，不删除数据；hourly/daily 使用 `retention_days`，monthly 使用 `monthly_retention_months`。
- `traffic timer install` 写 systemd service/timer 或 launchd plist，不启用、不启动。
- `traffic timer enable` 启用并启动自动调度；`disable` 停止并禁用自动调度。
- `traffic timer run` 不调用服务管理器，等价于立即执行对应 `collect ... --instance ALL`。

## 4. `sboxsub`

### 4.1 配置与 input

```bash
sboxsub [--base-dir DIR] init [--force]
sboxsub version
sboxsub [--base-dir DIR] config [--editor CMD]
sboxsub [--base-dir DIR] config show [--show-secrets]
sboxsub [--base-dir DIR] config check
sboxsub [--base-dir DIR] import BUNDLE [--replace-all]
sboxsub [--base-dir DIR] clear
sboxsub [--base-dir DIR] input list
sboxsub [--base-dir DIR] input show SOURCE [--raw] [--show-secrets]
sboxsub [--base-dir DIR] input validate [SOURCE]
sboxsub [--base-dir DIR] input edit SOURCE [--editor CMD]
sboxsub [--base-dir DIR] input clone SOURCE TARGET [--editor CMD]
sboxsub [--base-dir DIR] input set-host HOST [SOURCE] [--all]
sboxsub [--base-dir DIR] input remove SOURCE
```

- `sboxsub` 只读取自己的 `config.yaml` 和 `inputs/`。
- `import` 必须先完整校验 bundle、manifest、hash、schema 和路径安全，再原子写入。
- `show` 默认脱敏，只有 `--show-secrets` 输出敏感字段明文。

### 4.2 HTTP 与服务生命周期

```bash
sboxsub [--base-dir DIR] [--listen HOST:PORT] serve [--host HOST] [--port PORT]
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] service install
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] service uninstall
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] start
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] stop
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] restart
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] status
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] logs [--follow|-f]
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] enable
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] disable
sboxsub [--base-dir DIR] [--service-manager auto|systemd|launchd] doctor
```

- `serve` 启动阶段 input 非法则失败；运行期 reload 失败保留上一份可用 index。
- 订阅 HTTP 路由同时支持 `/clash/:user` 和 `/clash/:token/:user` 等形式；token 模式下 path token 优先，也允许 `?token=`。
- `service install` 只安装订阅服务自身的 unit 或 plist。
- 生命周期命令不得读取 agent 配置，不得读写 agent runtime。
