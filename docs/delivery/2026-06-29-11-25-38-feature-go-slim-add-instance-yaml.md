# 功能交付：精简 add 生成的 instance YAML 正文

## 任务背景

`sboxctl add` 生成的 instance 配置正文会展开大量空字段，例如 `transport`、`tls.enabled: false`、`auth.type: noauth`、空用户字段和空列表。用户编辑时容易误以为每种协议都需要填写这些字段。

## 实现方案

- 保持顶部注释区作为“支持协议和字段全集”的参考模板。
- 将未注释正文作为“当前模板可直接使用的协议子集”，只保留实际生效字段。
- 在写 instance YAML 前对 YAML node 做编辑版瘦身，不修改领域模型结构和运行时生成逻辑。

## 文件变更

- `internal/instance/manager.go`
  - 新增可编辑 YAML 瘦身逻辑。
  - 补齐顶部协议模板中 transport、VMess alter_id/security、用户 remark/tag 等字段示例。
- `internal/instance/manager_test.go`
  - 增加正文不包含空 `transport`、空 `tls`、默认 `noauth`、空列表等噪声字段的断言。

## 验证结果

```bash
go test ./internal/instance
go test ./...
git diff --check
```

以上命令均已通过。

## 风险与后续建议

- 瘦身只作用于写给用户编辑的 instance YAML，不改变加载、校验和 sing-box 生成语义。
- 若后续新增协议字段，需要同步确认顶部注释全集是否补充字段示例。
