# T09 Release、安装脚本与 Makefile 交付文档

## 任务背景

T09 目标是完成本地构建、tag release、二进制安装脚本和发布资产校验链路。范围限定为 `Makefile`、`.github/workflows/release.yml`、`scripts/install.sh`、`scripts/install-local.sh`、release 包结构、checksum 和对应验收测试。

## 实现方案

- 保留既有 Makefile 和 release workflow 契约：tag `v*.*.*` 触发、矩阵构建 Linux/macOS amd64/arm64、生成 `checksums.txt` 并通过 `gh release create` 上传资产。
- 加强 `scripts/install.sh` 的输入和归档安全校验：`--repo`、`--version` 增加白名单约束，归档成员先检查绝对路径、`..` 和反斜杠，再进入允许列表。
- 要求 release 归档同时包含 `bin/sboxctl` 和 `bin/sboxsub`，避免缺失资产进入安装流程。
- 调整远程安装 dry-run 行为：dry-run 只输出计划，不创建安装目录或临时下载目录。
- 调整 `--tmp-dir` 行为：指定临时目录时在其下创建受管 `mktemp` 子目录，并拒绝 `/`，避免清理用户传入目录本身。
- 调整 `--no-checksum` 行为：显式跳过 checksum 下载和校验，仅用于受控离线场景。
- 加强 `scripts/install-local.sh`：从本地 tar.gz dry-run 时只校验归档并输出安装计划，不解压；安装目标为目录时拒绝覆盖。

## 文件与配置变更

- 修改 `scripts/install.sh`：补输入白名单、dry-run、临时目录、checksum 跳过和归档成员校验顺序。
- 修改 `scripts/install-local.sh`：补 tar.gz dry-run 安装计划和目标目录保护。
- 修改 `internal/acceptance/release_install_test.go`：补远程安装 dry-run 不创建临时目录、远程安装拒绝路径穿越归档的验收覆盖。
- 修改 `docs/PROGRESS.md`：标记 T09 完成。
- 未新增依赖，未修改 Go 业务模块，未修改配置 schema。

## 测试结果

- `make help`：通过。
- `make test`：通过。
- `go test ./internal/acceptance`：通过。
- `shellcheck scripts/install.sh scripts/install-local.sh`：通过。
- `make lint`：通过目标执行；当前环境缺少 `golangci-lint`，按 Makefile 设计输出提示并跳过可选 lint。
- `make clean && make build && make package GOOS=linux GOARCH=amd64 VERSION=v0.0.0-test && make package GOOS=darwin GOARCH=arm64 VERSION=v0.0.0-test && make checksums`：通过。
- `tar -tzf dist/release/sbox-manager_v0.0.0-test_linux_amd64.tar.gz`：包结构符合 `bin/sboxctl`、`bin/sboxsub`、`README.md`。
- `tar -tzf dist/release/sbox-manager_v0.0.0-test_darwin_arm64.tar.gz`：包结构符合 `bin/sboxctl`、`bin/sboxsub`、`README.md`。
- `scripts/install-local.sh --from dist/release/sbox-manager_v0.0.0-test_darwin_arm64.tar.gz --install-dir <tmp>/bin --force`：通过，两个二进制 `version` 均为 `v0.0.0-test`。
- `make install-local BINDIR=<tmp>/bin`：通过，两个二进制 `version` 均为 `v0.0.0-test`。
- `scripts/install.sh --version v0.0.0 --repo owner/repo --os linux --arch amd64 --install-dir <tmp>/bin --tmp-dir <tmp>/tmp --dry-run`：通过，未创建安装目录和临时目录。

## 评审问题清单与处理结果

| 问题 | 级别 | 处理 |
| --- | --- | --- |
| `scripts/install.sh` 的归档成员允许列表先于 `..` 检查，`templates/../evil` 可能被允许列表命中 | 阻断 | 已改为先检查危险路径，再检查允许列表，并新增远程安装验收测试 |
| 远程安装 dry-run 会进入临时目录准备流程 | 一般 | 已改为 dry-run 在下载和临时目录准备前返回 |
| 用户传入 `--tmp-dir` 时脚本清理目标目录本身，误传大目录存在破坏风险 | 一般 | 已改为在传入目录下创建受管 `mktemp` 子目录，只清理受管子目录 |
| `--no-checksum` 仍下载 `checksums.txt` | 一般 | 已改为跳过 checksum 下载和校验 |

## 风险与后续建议

- GitHub Release workflow 尚未在真实 tag push 环境执行；本次完成静态契约检查和本地打包验证。
- `make lint` 依赖本机是否安装 `golangci-lint`；当前环境未安装，因此只验证了 Makefile 的缺失提示路径。
- release 资产当前只包含 `README.md` 和二进制；仓库暂无 `LICENSE` 或 `templates/` 时不会额外打包这些可选文件。
