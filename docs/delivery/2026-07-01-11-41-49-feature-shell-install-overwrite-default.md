# install.sh 默认覆盖策略交付说明

## 任务背景

- 调整远程安装脚本的覆盖策略：默认覆盖已存在的 `sboxctl` 和 `sboxsub`。
- 移除 `--force` 参数，新增 `--no-overwrite` 表示不覆盖已有二进制。

## 实现方案

- 将 `scripts/install.sh` 的参数从 `--force` 替换为 `--no-overwrite`。
- 默认安装时允许覆盖普通目标文件，仍拒绝覆盖目录。
- 新增安装目标预检查：真实安装前统一检查 `sboxctl` 和 `sboxsub`，避免 `--no-overwrite` 模式下只写入一半。
- `install_binary` 写入前保留同样保护，防止预检查后目标被并发创建。

## 文件变更

- 修改 `scripts/install.sh`：默认覆盖、支持 `--no-overwrite`、移除 `--force`。
- 修改 `internal/acceptance/release_install_test.go`：新增默认覆盖和 `--no-overwrite` 拒绝覆盖测试。
- 修改 `docs/release.md`：同步安装参数和行为说明。

## 配置与依赖变更

- 无新增依赖。
- 无新增配置项。
- 无数据库或外部服务变更。

## 测试结果

- `bash -n scripts/install.sh` 通过。
- `shellcheck scripts/install.sh` 通过。
- `go test ./internal/acceptance` 通过。
- `go test ./...` 通过。

## 风险与后续建议

- `--force` 已从 `install.sh` 移除，继续传入会按未知参数失败。
- `scripts/install-local.sh` 暂未调整，仍保持原有 `--force` 语义；如需本地安装脚本同样默认覆盖，可单独同步。
