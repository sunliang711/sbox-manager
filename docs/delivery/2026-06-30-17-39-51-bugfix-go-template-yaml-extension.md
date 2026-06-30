# 共享模板 YAML 静态解析报错修复

## 问题背景

`internal/configtemplate/templates` 下的模板源文件使用 `.yaml` 扩展名，但文件内容包含 Go template 动作，例如 `{{ .VmessOutboundName }}`、`{{ if .ShowExampleFields }}` 和 `{{ snippet ... }}`。当 YAML 校验器直接扫描这些文件时，会在模板渲染前把它们当普通 YAML 解析，导致 `found unhashable key`、`could not find expected ':'` 等错误。

## 根因分析

- 根因位置：`internal/configtemplate/templates/**/*.yaml`
- 问题类型：模板源文件扩展名与实际内容不匹配
- 触发条件：YAML 工具直接解析未渲染的 Go template 源文件
- 发生原因：`.yaml` 扩展名让静态工具误认为文件本身应为合法 YAML

## 修复方案

- 将共享模板源统一重命名为 `.yaml.tmpl`。
- 同步更新 `internal/configtemplate/templates.go` 中所有 embed registry 路径。
- 保持运行时渲染结果不变，`add` 和 `example` 仍输出 YAML。

## 验证结果

- `go test ./internal/configtemplate ./internal/instance ./internal/cli`
- `go test ./...`
- `find internal/configtemplate/templates -type f -name '*.yaml' -print` 无输出

## 风险与后续

- 该修复只改变模板源文件扩展名和 registry 路径，不改变渲染后的配置结构。
- 若后续新增共享模板，应继续使用 `.yaml.tmpl`，避免静态 YAML 校验误扫模板源。
