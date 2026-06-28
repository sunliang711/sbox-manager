# T09 Release、安装脚本与 Makefile

## 目标

建立本地构建、tag release、二进制安装脚本和发布资产校验的完整交付链路。

## 范围

- 根目录 `Makefile`。
- `.github/workflows/release.yml`。
- `scripts/install.sh`。
- `scripts/install-local.sh`。
- release 包结构。
- `checksums.txt`。
- release dry-run 和安装脚本测试。

## 技术方案

Makefile 提供 `help`、`fmt`、`lint`、`test`、`build`、`snapshot`、`package`、`checksums`、`install-local`、`clean` 等目标。

GitHub Actions 只在 `v*.*.*` tag push 时创建正式 release。工作流构建 Linux/macOS 的 amd64/arm64 资产，运行测试，生成 sha256，创建 GitHub Release 并上传 `.tar.gz` 和 `checksums.txt`。

安装脚本只安装 `sboxctl`、`sboxsub` 二进制，不初始化配置、不安装服务、不启动进程。

详细契约以 `docs/release.md` 为准。

## 验收标准

- `make help` 可用且不产生副作用。
- `make test` 执行 `go test ./...`。
- `make build` 在当前平台生成 `sboxctl` 和 `sboxsub`。
- `make package GOOS=linux GOARCH=amd64 VERSION=v0.0.0-test` 生成符合命名和目录结构的压缩包。
- `make checksums` 生成 sha256 校验文件。
- release workflow 仅对 `v*.*.*` tag 触发。
- release workflow 包含 `contents: write` 权限、测试步骤、矩阵构建、checksum 生成和 release 上传。
- 安装脚本默认校验 checksum，拒绝路径穿越归档。
- 安装脚本不会创建配置目录、安装服务或启动进程。
- `install-local.sh` 支持从本地 tar.gz 和 `dist/bin` 安装。

## 风险

- tag release 失败后可能留下半成品 release。工作流必须避免静默覆盖既有资产，并在失败时清楚输出 tag 和资产名。
