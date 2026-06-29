# sbox-manager

`sbox-manager` 是围绕 sing-box 构建的代理管理工具集，提供实例管理、订阅服务、流量统计、诊断、备份恢复和发布安装链路。

项目包含两个二进制：

- `sboxctl`：管理 sing-box 实例、配置生成、服务生命周期、sing-box/rules 安装、订阅导出、流量统计、诊断和备份恢复。
- `sboxsub`：独立运行订阅服务，负责订阅 input 管理、bundle 导入、HTTP 订阅输出和订阅服务生命周期。

本项目参考 `proxystack-go` 和 `xray traffic` 的功能与运维经验，但使用新的 sing-box 配置模型；不兼容旧配置、旧 CLI、旧订阅包或旧 SQLite 数据库。

## 功能概览

- 单 sing-box core 的多实例管理。
- systemd 和 launchd 服务文件生成与生命周期管理。
- VMess、VLESS、AnyTLS、Shadowsocks、Socks、HTTP、sing-box native 等订阅节点输入与输出。
- Clash、Premium Clash、Surge、sing-box 订阅输出。
- 基于 sing-box stats API 的流量采集、小时/日/月聚合、年度展示、CSV 导出和保留期清理。
- `doctor`、`ipinfo`、配置备份与恢复。
- Makefile 本地构建、release 打包、checksum 生成和二进制安装脚本。

## 安装

从 GitHub Release 安装二进制：

```bash
curl -fsSLO https://raw.githubusercontent.com/sunliang711/sbox-manager/main/scripts/install.sh
bash install.sh --version vX.Y.Z
```

安装脚本只安装 `sboxctl` 和 `sboxsub`，不会初始化配置、安装服务或启动进程。默认会下载 `checksums.txt` 并校验 release 包。

本地构建并安装：

```bash
make build
make install-local BINDIR="$HOME/.local/bin"
```

从本地 release 包安装：

```bash
make package GOOS=linux GOARCH=amd64 VERSION=v0.0.0-test
scripts/install-local.sh --from dist/release/sbox-manager_v0.0.0-test_linux_amd64.tar.gz --install-dir "$HOME/.local/bin"
```

## 本地开发

常用 Makefile 目标：

```bash
make help
make fmt
make lint
make test
make build
make package GOOS=linux GOARCH=amd64 VERSION=v0.0.0-test
make checksums
make clean
```

构建产物默认输出到 `dist/`。`build` 和 `package` 会通过 ldflags 注入版本、commit 和构建时间。

## 快速开始

准备 agent 本机配置和服务文件：

```bash
sudo sboxctl --base-dir /opt/sbox-manager setup local --external-host proxy.example.com
```

查看配置示例：

```bash
sboxctl example global
sboxctl example instance edge
sboxctl example inbound vmess
```

添加并校验实例：

```bash
sudo sboxctl --base-dir /opt/sbox-manager add edge-us --template edge --no-edit
sudo sboxctl --base-dir /opt/sbox-manager validate edge-us
sudo sboxctl --base-dir /opt/sbox-manager check edge-us
```

下载运行资源并启动实例：

```bash
sudo sboxctl --base-dir /opt/sbox-manager setup binary
sudo sboxctl --base-dir /opt/sbox-manager --service-manager auto start edge-us
sudo sboxctl --base-dir /opt/sbox-manager --service-manager auto status edge-us
```

初始化订阅服务并导入 agent 导出的 bundle：

```bash
sboxsub --base-dir /opt/sbox-sub init
sboxctl --base-dir /opt/sbox-manager sub export edge-us -o /tmp/sbox-sub-bundle.tar.gz
sboxsub --base-dir /opt/sbox-sub import /tmp/sbox-sub-bundle.tar.gz
sboxsub --base-dir /opt/sbox-sub config check
sboxsub --base-dir /opt/sbox-sub serve
```

## 常用命令

实例与配置：

```bash
sboxctl list --verbose
sboxctl config edge-us
sboxctl render sing-box edge-us
sboxctl restart edge-us
sboxctl logs edge-us --follow
```

诊断、安装和备份：

```bash
sboxctl doctor
sboxctl ipinfo edge-us
sboxctl setup binary
sboxctl export -o backup.tar.gz
sboxctl import backup.tar.gz --force
```

流量统计：

```bash
sboxctl traffic collect hourly --instance ALL
sboxctl traffic show current --instance ALL
sboxctl traffic show daily --instance edge-us --days 7
sboxctl traffic summarize monthly --instance ALL
sboxctl traffic export daily --instance ALL --format csv --output traffic.csv
sboxctl traffic cleanup records --period all --dry-run
```

订阅服务：

```bash
sboxsub input list
sboxsub input validate
sboxsub input show SOURCE
sboxsub service install
sboxsub start
sboxsub status
```

## 默认目录

- `sboxctl --base-dir` 默认：`/opt/sbox-manager`
- `sboxsub --base-dir` 默认：`/opt/sbox-sub`

建议目录结构：

```text
/opt/sbox-manager/
  bin/
  config.yaml
  instances/
  runtime/
  publish/
  traffic/
  rules/
  downloads/
  logs/

/opt/sbox-sub/
  config.yaml
  inputs/
  templates/
  logs/
```

## 安全边界

- `sboxsub` 默认建议监听 loopback；公网部署必须显式配置监听地址和 token。
- 公开 socks/http inbound 默认要求鉴权。
- 安装脚本不执行 `setup`、不安装 systemd/launchd 服务、不启动进程。
- release 归档安装前会校验 checksum，并拒绝绝对路径、`..`、反斜杠路径和未知成员。
- 日志和默认展示会避免输出 token、password、secret、UUID 明文和完整订阅内容。

## 文档

- [总体架构方案](docs/architecture.md)
- [CLI 命令规格](docs/cli-spec.md)
- [配置与数据规格](docs/data-spec.md)
- [发布、安装与 Makefile 规格](docs/release.md)
- [开发规范](docs/conventions.md)
- [验收矩阵](docs/acceptance-matrix.md)
- [进度跟踪](docs/PROGRESS.md)
- [任务拆分](docs/tasks/)
