# sboxctl config 校验失败保留草稿修复

## 问题背景

执行 `sboxctl config usa1` 编辑实例配置时，如果保存后的内容没有通过校验，命令会返回错误，但刚才在编辑器里修改的内容会被删除，用户只能重新编辑。

## 根因分析

`editConfigCommand` 和 `editInstanceByName` 在编辑前会把正式配置复制到 `*.draft`，并使用 `defer os.Remove(draft)` 无条件清理草稿。校验失败时函数直接返回错误，导致保存后的草稿也被删除。

## 修复方案

- 编辑开始时如果已经存在 `*.draft`，优先复用该草稿，避免覆盖上次校验失败后保留的内容。
- 校验失败时不删除草稿，并在错误信息中提示 `edited draft saved at <path>`。
- `add` / `clone` 后编辑失败时，错误信息提示实例文件已经创建，并建议继续执行 `sboxctl config NAME` 修复保留的草稿。
- 校验通过或确认配置无变化后才清理草稿。
- 正式配置仍然只在校验通过后替换，失败时不会污染原文件。

## 文件变更

- `internal/cli/sboxctl_t04.go`
  - 新增 `prepareEditableDraft` 和 `preservedDraftError`。
  - 新增 `editableInstanceRecoveryError`，用于 `add` / `clone` 后编辑失败时提示恢复命令。
  - 调整 `editConfigCommand`、`editInstanceByName` 的草稿清理时机。
- `internal/cli/command_test.go`
  - 新增校验失败保留草稿测试。
  - 新增 `add` 后编辑校验失败保留草稿测试。
  - 新增再次编辑复用保留草稿测试。

## 验证结果

- `go test ./internal/cli`
- `go test ./...`

## 风险与说明

本次修复限定在 `sboxctl config` 的 agent 全局/实例配置编辑路径。`sboxsub config`、`traffic edit config` 也有类似草稿模式，但未在本次需求范围内修改。
