# 发布、安装与 Makefile 规格

本文定义本项目的本地构建入口、GitHub tag release 自动化、二进制安装脚本和发布产物契约。

## 1. 目标

- 本地开发者可以通过 `make` 完成测试、构建、打包和清理。
- 推送形如 `vX.Y.Z` 的 tag 后，GitHub Actions 自动构建 release。
- release 中提供 Linux/macOS 常用架构的 `sboxctl`、`sboxsub` 二进制压缩包和校验文件。
- 用户可以通过安装脚本只安装二进制，不隐式初始化配置、不安装服务、不启动进程。

## 2. Makefile

仓库根目录必须提供 `Makefile`。默认目标为 `help`，不得产生副作用。

必备 target：

| target | 说明 |
| --- | --- |
| `help` | 展示常用目标 |
| `fmt` | 执行 `gofmt`/`go fmt` |
| `lint` | 执行静态检查；工具缺失时给出安装提示 |
| `test` | 执行 `go test ./...` |
| `build` | 构建当前平台 `sboxctl` 和 `sboxsub` 到 `dist/bin/` |
| `snapshot` | 构建当前 git 状态的本地快照包 |
| `package` | 为指定 `GOOS/GOARCH` 生成 release 压缩包 |
| `checksums` | 对 `dist/` 下 release 资产生成 `checksums.txt` |
| `install-local` | 将本地构建出的二进制安装到指定目录 |
| `clean` | 删除 `dist/`、临时包和测试缓存 |

变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `VERSION` | `dev` 或当前 tag | 注入 `internal/version` |
| `COMMIT` | `git rev-parse --short HEAD` | 注入 `internal/version` |
| `BUILD_TIME` | UTC RFC3339 | 注入 `internal/version` |
| `GOOS` | 当前平台 | 交叉编译目标 OS |
| `GOARCH` | 当前架构 | 交叉编译目标 arch |
| `PREFIX` | `/usr/local` | `install-local` 前缀 |
| `BINDIR` | `$(PREFIX)/bin` | 二进制安装目录 |

约束：

- `build`、`package` 必须通过 `-ldflags` 注入 version、commit、build time。
- `package` 只打包二进制、README、LICENSE 和必要模板，不包含配置、runtime、traffic DB 或本地下载缓存。
- `install-local` 只安装 `sboxctl`、`sboxsub`，不运行 `sboxctl setup` 或 `service install`。
- `clean` 只删除项目内生成目录，不删除用户配置目录。

## 3. GitHub Actions Release

工作流路径：`.github/workflows/release.yml`。

触发条件：

```yaml
on:
  push:
    tags:
      - "v*.*.*"
```

构建矩阵：

| GOOS | GOARCH | 产物 |
| --- | --- | --- |
| `linux` | `amd64` | `sbox-manager_<version>_linux_amd64.tar.gz` |
| `linux` | `arm64` | `sbox-manager_<version>_linux_arm64.tar.gz` |
| `darwin` | `amd64` | `sbox-manager_<version>_darwin_amd64.tar.gz` |
| `darwin` | `arm64` | `sbox-manager_<version>_darwin_arm64.tar.gz` |

构建元信息：

- `VERSION` 必须使用当前 tag，即 `${GITHUB_REF_NAME}`。
- `COMMIT` 必须使用 `git rev-parse --short HEAD`。
- `BUILD_TIME` 必须使用 UTC RFC3339 时间。

流程：

1. checkout 完整 tag 历史。
2. 安装指定 Go 版本。
3. 执行 `go test ./...`。
4. 按矩阵执行 `make package GOOS=<os> GOARCH=<arch> VERSION=<tag>`。
5. 生成 `checksums.txt`。
6. 创建 GitHub Release，并上传所有 `.tar.gz` 和 `checksums.txt`。

权限：

```yaml
permissions:
  contents: write
```

约束：

- 只有 tag push 触发正式 release，不从普通 branch push 创建 release。
- release 标题使用 tag 名，release notes 可由 GitHub 自动生成。
- 资产上传前必须先生成 checksum。
- 如果 release 已存在，工作流必须失败或显式 dry-run，不得静默覆盖既有资产。
- 发布失败不得修改仓库内容或重新打 tag。

## 4. Release 资产

压缩包内部结构：

```text
sbox-manager_<version>_<os>_<arch>/
  bin/
    sboxctl
    sboxsub
  README.md
  LICENSE
```

可选文件：

```text
  templates/
```

`checksums.txt`：

```text
<sha256>  sbox-manager_<version>_<os>_<arch>.tar.gz
```

约束：

- checksum 使用 sha256。
- 压缩包内路径必须是相对路径，不允许绝对路径、`..` 或反斜杠。
- 二进制权限为 `0755`。
- 压缩包不包含 `.git`、`dist/`、本地配置、runtime、traffic DB、downloads、logs。

## 5. 安装脚本

脚本路径：

- `scripts/install.sh`：从 GitHub Release 下载并安装二进制。
- `scripts/install-local.sh`：从本地 release 包或 `dist/bin` 安装二进制。

`scripts/install.sh` 用法：

```bash
curl -fsSLO https://raw.githubusercontent.com/sunliang711/sbox-manager/main/scripts/install.sh
bash install.sh --version vX.Y.Z
```

不推荐直接使用 `curl | sh`。需要非交互安装时，应先下载脚本并检查内容或固定到可信 tag。

支持参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--version V` | latest release | 指定 release tag |
| `--repo OWNER/REPO` | 项目仓库 | 指定 GitHub 仓库 |
| `--os OS` | 自动探测 | `linux`、`darwin` |
| `--arch ARCH` | 自动探测 | `amd64`、`arm64` |
| `--install-dir DIR` | `/usr/local/bin` | 二进制安装目录 |
| `--tmp-dir DIR` | mktemp | 临时下载目录 |
| `--dry-run` | false | 只展示操作 |
| `--force` | false | 覆盖已存在二进制 |
| `--no-checksum` | false | 禁用 checksum 校验，仅用于受控离线场景 |

行为：

- 自动探测 OS/arch 并映射到 release asset 名。
- 下载 `.tar.gz` 和 `checksums.txt`。
- 默认必须校验 sha256。
- 解压时拒绝绝对路径、`..`、反斜杠路径和未知成员。
- 安装前先写临时文件，设置权限后原子 rename 到目标路径。
- 已存在非受管文件且未传 `--force` 时失败。
- 安装完成后执行 `sboxctl version` 和 `sboxsub version` 做轻量验证。

禁止行为：

- 不创建 `/opt/sbox-manager` 或 `/opt/sbox-sub`。
- 不写 agent/sub 配置。
- 不安装 systemd/launchd 服务。
- 不启动或停止任何服务。
- 不自动执行 `setup`。

`scripts/install-local.sh` 用于开发和离线安装：

```bash
scripts/install-local.sh --from dist/release/sbox-manager_vX.Y.Z_linux_amd64.tar.gz --install-dir /usr/local/bin
scripts/install-local.sh --from dist/bin --install-dir ./tmp/bin
```

## 6. 官方资料依据

- GitHub Actions 支持在 `push.tags` 下按 tag 过滤触发工作流：`https://docs.github.com/actions/using-workflows/workflow-syntax-for-github-actions`。
- GitHub Release 用于发布软件包、release notes 和二进制文件：`https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases`。
- GitHub CLI `gh release create` 可创建 release 并附加资产：`https://cli.github.com/manual/gh_release_create`。
- GitHub CLI `gh release upload` 可向 release 上传资产：`https://cli.github.com/manual/gh_release_upload`。
