# T03 sing-box 生成器与 Runtime 交付记录

## 任务背景

- 目标：从新领域模型生成稳定 sing-box JSON，并实现 Runtime Plan、manifest 和原子写入。
- 范围：sing-box 生成器、订阅 input 基础生成、runtime manifest/diff/apply、`validate`、`check`、`render`、`export-config`、`start`、`restart` 的 T03 最小可用接入。
- 边界：真实 systemd/launchd 服务管理、服务文件安装、完整订阅 HTTP 服务和 traffic DB 仍留给后续任务。

## 多 Agent 编排

- 开发 agent：实现生成器、runtime、CLI 接入与测试。
- review agent：独立审查 T03 实现，首轮发现 1 个阻断问题。
- 开发 agent 复修一：修复缺失 `sing-box` 二进制时静默跳过检查的问题。
- review agent 复审二：继续发现 nil checker 会退回 noop 的阻断问题。
- 开发 agent 复修二：收紧 nil checker 语义，并补充无副作用回归测试。
- review agent 复审三：阻断问题清零，T03 通过。

## 实现摘要

- 新增 `internal/generator/singbox`：
  - 将 `GlobalConfig` 与 `Instance` 纯函数转换为稳定 sing-box JSON。
  - 保持顶层输出顺序为 `log`、`dns`、`inbounds`、`outbounds`、`route`、`experimental`。
  - 支持基础 inbound/outbound/group/route/API experimental 映射。
  - 生成订阅 input，并提供 `export-config` 的基础输出能力。
- 新增 `internal/runtime`：
  - 实现 Runtime Manifest、严格读取、路径安全校验和稳定排序。
  - 实现 `create`、`update`、`delete`、`no-change` diff plan。
  - 实现同目录临时文件、fsync、rename 的原子写入。
  - no-change 时不改写 generated 或 manifest。
  - 提供可注入 `ConfigChecker` 和 `ServiceManager`，start/restart 遵循 checker -> apply -> service manager 顺序。
- 新增 `internal/config/set.go`：
  - 只读加载 agent 全局配置和实例配置集合。
- 更新 `internal/cli`：
  - 接入 `validate`、`check`、`render model`、`render sing-box`、`render sub`、`export-config`、`start`、`restart`。
  - `check`、`render` 和 `export-config` 保持只读输出。
- `docs/PROGRESS.md` 将 T03 标记为已完成。

## 评审问题与处理

| 问题 | 处理结果 |
| --- | --- |
| 默认 `CommandConfigChecker` 找不到 `sing-box` 时静默通过，导致 `start/restart` 可未经真实检查写 runtime | 已改为返回明确错误；补充 checker 失败时不写 runtime、不调用 service manager 的测试 |
| `CheckPlan` 在 nil checker 时退回 `NoopConfigChecker`，内部 API 仍可跳过检查 | 已改为 nil checker 返回错误；只有显式注入 `NoopConfigChecker{}` 才跳过检查；补充 start/restart nil checker 无副作用测试 |

## 验证结果

- `go test ./...` 通过。
- `make build` 通过。
- review 三次复审通过，阻断问题为“无”。
- 当前环境未发现 `sing-box` 二进制，因此未执行真实 `sing-box check -c` 端到端抽测。

## 风险与后续建议

- 后续在安装真实 `sing-box` 的环境中补一次端到端验收，确认生成文件可通过真实 `sing-box check -c`。
- T04 接入真实 systemd/launchd 服务管理时，应复用当前 `ServiceManager` 接口，并继续保持测试中无系统服务副作用。
