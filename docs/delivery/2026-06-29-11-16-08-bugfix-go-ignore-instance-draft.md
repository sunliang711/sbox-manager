# Bug 修复交付：忽略 instance 草稿文件

## 问题背景

执行 `sboxctl list` 时，如果 `instances/` 下存在编辑草稿，例如 `usa4.draft.yaml`，加载实例列表会报错：

```text
文件名 "usa4.draft" 必须与 instance name "usa4" 一致
```

## 根因分析

- 旧草稿命名会把 `usa4.yaml` 生成为 `usa4.draft.yaml`。
- `LoadInstances` 只根据最后扩展名判断 `.yaml/.yml/.json`，因此会把 `usa4.draft.yaml` 当成正式 instance 配置加载。
- `LoadInstance` 对 YAML 文件校验文件名与 `name` 一致，草稿文件名 `usa4.draft` 与配置内 `name: usa4` 不一致，触发错误。

## 修复方案

- 草稿路径改为在原文件名后追加 `.draft`，例如 `usa4.yaml.draft`。
- 配置格式识别支持剥离 `.draft` 后缀后再判断原始扩展名。
- `LoadInstances` 忽略包含 `.draft`、`.tmp`、`.swp` 的临时文件，兼容旧式 `usa4.draft.yaml` 和新式 `usa4.yaml.draft`。

## 文件变更

- `internal/cli/sboxctl_t04.go`
- `internal/config/load.go`
- `internal/config/set.go`
- `internal/config/load_test.go`
- `internal/cli/command_test.go`

## 验证结果

```bash
go test ./internal/config ./internal/cli
go test ./...
git diff --check
```

以上命令均已通过。

## 风险与后续建议

- 该修复只影响草稿文件命名、配置格式识别和 instance 目录过滤。
- 已兼容历史遗留的 `*.draft.yaml` 草稿文件，无需用户手动清理后才能执行 `list`。
