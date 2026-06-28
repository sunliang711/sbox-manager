# T05 订阅服务交付说明

## 任务背景

- 目标：实现新订阅 schema、bundle、订阅渲染和 `sboxsub` HTTP 服务。
- 范围：订阅 input 校验、index 构建、bundle export/import、Clash/Premium Clash/Surge/sing-box 渲染、HTTP 服务、watcher reload、T05 CLI 入口。
- 边界：未实现 T06-T09；未新增第三方依赖；未改动 agent runtime、traffic、release 等后续任务范围。

## 已落地实现

- `internal/domain`：补齐订阅 input、bundle manifest、文件名、hash、access 和合并唯一性校验。
- `internal/config`：订阅 input 和 bundle manifest 加载后执行完整领域校验。
- `internal/subscription`：新增 input 目录加载、index 构建、脱敏展示、bundle zip 导出/导入、安全成员校验、hash 校验、原子写入/替换、模板渲染上下文和内置模板。
- `internal/subserver`：新增标准库 HTTP server、`/health`、订阅路由、token/none 鉴权、模板错误 503、watcher 轮询 reload，reload 失败保留旧 index。
- `internal/cli`：接入 `sboxctl sub export`、`sboxctl sub validate-inputs`、`sboxsub init/config/import/clear/input/serve` 的 T05 相关实现。
- 文档：更新 `docs/PROGRESS.md`，标记 T05 已完成。

## 测试结果

- 已补关键验收测试：
  - HTTP 401/403/404/503。
  - path token 优先于 query token。
  - reload 失败保留旧 index。
  - bundle 非法成员和 hash mismatch 不写入。
  - bundle `access.token` 不导入、不修改 sboxsub config。
  - `sboxctl sub export --dry-run` 和 `--summary` 不写 publish。
- 完整测试命令：

```bash
go test ./...
```

- 结果：通过。

## Review 与处理结果

- 首轮 review 发现 3 个阻断、3 个警告、1 个建议。
- 已修复 `sboxsub config`、`sboxsub input edit`、`input clone --editor` 的草稿编辑和整体校验流程。
- 已补齐 `sboxsub service install/uninstall`、`start/stop/restart/status/logs/enable/disable` 和 `doctor`。
- 已将默认 bundle import 改为合并后目录级 staging 替换，避免部分写入。
- 已补充格式级节点过滤、watcher 非普通文件跳过、reload 恢复后清空 `last_error`。
- 复审结论：通过，未发现新的阻断或警告问题。
