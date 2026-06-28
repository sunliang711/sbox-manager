# T06 流量统计交付记录

## 结论

- 状态：已完成，并通过独立 review agent 复审，最终结论为“无问题”。
- 范围：stats client、baseline delta、SQLite/GORM schema、hourly/daily/monthly collect、current/watch、show/summarize/export、cleanup、systemd/launchd traffic timer。
- 边界：T07-T09 未实现；真实环境仍需在部署验收阶段用实际 sing-box V2Ray API 做 smoke test。

## 主要实现

- 新增 `internal/traffic`：
  - V2Ray StatsService gRPC client，`QueryStats(reset=false)`。
  - stats name 解析、fixture JSON 解析。
  - baseline delta、计数回退 `reset_detected`。
  - GORM SQLite repository、WAL、busy timeout、schema version metadata。
  - hourly 写入、daily/monthly 聚合、yearly 动态聚合。
  - current 未落库增量、ALL 小计、summary、CSV、cleanup。
- 接入 `sboxctl traffic`：
  - `collect/show/watch/summarize/export/list/check/edit/cleanup/timer`。
  - show/summarize/export/check health 使用只读 DB 打开路径。
  - collect/cleanup 使用读写 DB。
- 新增 traffic timer：
  - systemd service/timer。
  - launchd plist。
  - install/uninstall/enable/disable/status/logs/run。

## Review 修复

- hourly record 写入和 baseline 更新改为同一 DB 事务，避免 delta 丢失。
- daily/monthly 聚合改为按窗口替换，源为空时清理旧聚合。
- launchd disable 会 disable 后 bootout，并对未加载状态幂等容错。
- 查询范围改为 overlap 语义，补齐 monthly/yearly 的 range 映射。
- metadata 高版本在 AutoMigrate 前拒绝打开。
- 只读命令不再隐式创建、迁移或写入 traffic DB。

## 验证

- `go test ./...`
- `go test -race ./internal/traffic ./internal/cli ./internal/service`
- `go mod tidy -diff`

说明：macOS race 测试期间出现系统 linker `LC_DYSYMTAB` warning，但测试返回码为 0。
