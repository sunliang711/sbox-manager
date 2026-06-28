# T06 流量统计

## 目标

实现参考 `/Users/eagle/.local/apps/init/tools/xray/traffic` 功能点的 Go 版 sing-box 流量统计。

## 范围

- stats client。
- baseline delta。
- SQLite schema。
- hourly/daily/monthly collect。
- current/watch。
- hourly/daily/monthly/yearly show。
- summarize。
- CSV export。
- list instances。
- check config/health。
- edit config。
- cleanup。
- retention 策略。
- systemd timer。
- launchd timer。

## 技术方案

不依赖 sing-box reset。每次采集读取当前累计值，和 baseline 相减得到增量。

SQLite 表结构、GORM 迁移策略、CSV 字段、timer service/plist 字段以 `docs/data-spec.md` 为准。

如果当前计数小于 baseline：

- 视为实例重启或计数 reset。
- delta 使用当前值。
- 记录 `reset_detected=true`。
- 更新 baseline。

## 验收标准

- `collect hourly --instance ALL` 支持多实例。
- daily 从 hourly 聚合，不调用 API。
- monthly 从 daily 聚合，不调用 API。
- yearly 从 monthly 动态聚合，不落库。
- show 当前周期追加未落库增量。
- watch 支持 interval、count、no-clear。
- show/watch/summarize/export 支持 scope/name 过滤。
- show/summarize/export 支持 date/from/to/days、month/months、year/years 范围参数。
- export 输出 CSV。
- list/check/edit 命令结构贴近 `xray-traffic`。
- cleanup 支持 period、dry-run 和删除计数输出。
- cleanup 按 hourly/daily/monthly 分别使用 data spec 定义的保留期。
- Linux 生成 systemd timer，macOS 生成 launchd 定时任务。
- `traffic timer install|uninstall|enable|disable|status|logs|run` 行为符合 `docs/cli-spec.md`。

## 风险

- sing-box stats 字段可能随版本变化。StatsClient 必须通过 fixture 和版本检测隔离风险。
