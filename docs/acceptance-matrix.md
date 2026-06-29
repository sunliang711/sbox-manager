# 验收矩阵

## 1. 配置与模型

| 场景 | 验收标准 |
| --- | --- |
| 全局配置加载 | 默认值完整，路径按 base dir 解析，非法 version 失败 |
| instance 配置加载 | 支持 enabled、role、labels、api、inbounds、outbounds、groups、route、traffic |
| 端口范围 | 支持 `start-end` 和结构化写法，冲突时报错 |
| 公开监听安全 | 非 loopback socks/http 无鉴权时默认失败 |
| 引用校验 | group、route、outbound 引用不存在时报错 |

## 2. sing-box 生成器

| 场景 | 验收标准 |
| --- | --- |
| VMess inbound | 多用户生成稳定 JSON，用户字段正确 |
| Shadowsocks inbound | method/password/udp 正确 |
| SOCKS inbound | noauth/password 两种模式正确 |
| HTTP inbound | 认证和监听正确 |
| direct outbound | 生成 sing-box direct |
| socks/http outbound | server、port、auth 正确 |
| selector group | 生成 selector outbound |
| urltest group | 生成 urltest outbound，url/interval 正确 |
| route rules | 使用 sing-box rule action，默认出站正确 |
| 稳定输出 | 相同输入多次输出字节级一致 |
| sing-box check | 生成后可通过 `sing-box check -c` |

## 3. Runtime Plan

| 场景 | 验收标准 |
| --- | --- |
| check | 只读，不创建 generated 或 manifest |
| start | 原子写 generated，写 manifest，调用 start；no-change 时不改写文件 |
| restart | 原子 apply runtime 后强制调用目标服务 restart；no-change 时不改写文件 |
| no-change | Runtime Plan 不改写 generated 或 manifest |
| delete | 删除 manifest 中失效生成文件 |
| target scope | 只影响目标 instance |

## 4. 实例 CLI

| 场景 | 验收标准 |
| --- | --- |
| init | 创建目录和默认配置，不覆盖既有文件 |
| setup | 依次执行 init、install all、service install，`--start` 时继续 start |
| add | 创建 instance 配置，端口自动分配 |
| example | 输出可复制 YAML 片段，保持只读 |
| clone | 新 instance 名和端口正确 |
| member | 维护 selector/urltest group 成员，普通 group 类型拒绝 |
| remove | 默认只删除或归档配置，`--purge` 才清理 generated |
| config | 草稿编辑，校验失败不覆盖原文件 |
| validate | 聚合输出配置错误 |
| render | 输出 model 或 sing-box JSON，保持只读 |
| export-config | 输出 Clash、Premium Clash、Surge、sing-box 订阅文本，保持只读 |
| list | 展示 instance、监听地址、服务状态 |
| service install | Linux 写 systemd unit，macOS 写 launchd plist，且不启动服务 |
| service files | systemd unit 和 launchd plist 包含 ExecStart/ProgramArguments、WorkingDirectory、User/Group、日志、权限和 hardening 约束 |

## 5. 安装更新卸载

| 场景 | 验收标准 |
| --- | --- |
| install sing-box | 下载、sha256 校验、原子安装 |
| update sing-box | 强制替换，失败回滚 |
| custom source | 自定义远端 source 缺少 `--sha256` 时失败 |
| install all | 不包含 self update |
| uninstall | 默认保留配置、traffic、publish |
| uninstall --purge | 删除受管目录和服务文件 |
| symlink | 不覆盖非受管文件 |

## 6. 订阅服务

| 场景 | 验收标准 |
| --- | --- |
| init | 创建 sub root、inputs、templates、config |
| input validate | 单文件和全量合并校验 |
| input edit | 草稿失败不覆盖原文件 |
| import bundle | 校验 manifest、hash、路径、安全成员 |
| bundle manifest | `manifest.json` 字段、`inputs_sha256` 集合和 zip 成员完全匹配 |
| serve | 启动前 input 非法则失败 |
| reload | 失败保留旧 index，health 返回 error |
| token | 缺失 401，错误 403 |
| token routes | none 模式支持无 token 路径，token 模式支持 path token 和 query token |
| not found | 用户不存在返回 404 |
| template error | 返回 503 |
| routes | Clash、Premium Clash、Surge、sing-box 输出正常 |
| sboxctl sub export | 生成订阅 bundle，`--dry-run` 和 `--summary` 保持只读 |

## 7. 流量统计

| 场景 | 验收标准 |
| --- | --- |
| list instances | 展示可采集实例和 API 地址 |
| collect hourly | 读取 stats，按 baseline 写 delta |
| collect date/month | daily 支持 `--date`，monthly 支持 `--month` |
| reset detected | 当前计数小于 baseline 时记录 reset_detected |
| collect daily | 从 hourly 聚合，不调用 API |
| collect monthly | 从 daily 聚合，不调用 API |
| show current | 展示当前未落库增量 |
| watch current | 支持 interval、count、no-clear |
| show hourly/daily/monthly | 历史记录加当前周期动态值 |
| show yearly | 从 monthly 动态聚合，不落库 |
| instance ALL | 输出跨实例小计 |
| filter options | show/watch/summarize/export 支持 `--scope` 和 `--name` |
| time range | show/summarize/export 支持 date/from/to/days、month/months、year/years |
| summarize | 输出范围、实例、scope、name 和汇总表 |
| export | CSV 字段固定为 data spec 定义字段 |
| cleanup | 支持 `--period hourly|daily|monthly|all`、`--dry-run`、hourly/daily/monthly 保留期和删除计数输出 |
| check config/health | 检查 traffic 配置、数据库和采集目标 |
| edit config | 草稿编辑，校验失败不覆盖原文件 |
| timer install | Linux 写 systemd service/timer，macOS 写 launchd plist，并启用 timer |
| timer uninstall | 停用并删除受管 service/timer 或 plist，不删除 traffic DB |
| timer enable/disable | 启用或禁用 hourly/daily/monthly 自动调度 |
| timer status/logs | 展示调度状态和日志，支持 `--follow` |
| timer run | 同步执行对应 collect 命令，不调用服务管理器 |

## 8. 诊断备份

| 场景 | 验收标准 |
| --- | --- |
| doctor | 检查目录、权限、二进制、服务、配置、API、traffic |
| ipinfo | 通过本地 socks/http listener 查询出口 IP |
| export | 只包含新 agent 配置和 `backup_manifest.json`，不包含 runtime、traffic、downloads、logs，不接受订阅 bundle |
| import | 校验 `backup_manifest.json`、hash/schema 后写入，不接受订阅 bundle |
| secret masking | 默认不输出敏感字段 |

## 9. 端到端

| 场景 | 验收标准 |
| --- | --- |
| 单节点实例 | init/add/check/start/status/logs/stop 全流程通过 |
| 订阅发布 | sboxctl sub export 后 sboxsub import/serve 可获取订阅 |
| traffic 定时 | hourly/daily/monthly timer install/enable/disable/status/run/logs/uninstall 可独立执行 |
| 故障恢复 | 配置错误不会覆盖上一份可用 runtime |
| Linux systemd | service install/start/status/logs/stop 覆盖 systemd |
| macOS launchd | service install/start/status/logs/stop 覆盖 launchd |

## 10. 发布与安装

| 场景 | 验收标准 |
| --- | --- |
| Makefile | `help/test/build/package/checksums/install-local/clean` 可用，默认 target 不产生副作用 |
| build metadata | release 和本地 build 均注入 version、commit、build time |
| tag release | 推送 `v*.*.*` tag 后触发 GitHub Actions release workflow |
| release assets | Linux/macOS amd64/arm64 压缩包和 `checksums.txt` 命名、结构、权限正确 |
| checksum | 安装脚本默认校验 sha256，校验失败不安装 |
| install.sh | 只安装 `sboxctl`、`sboxsub` 二进制，不初始化配置、不安装服务、不启动进程 |
| install-local.sh | 支持从本地 tar.gz 或 `dist/bin` 安装 |
