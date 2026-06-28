# T07 诊断、ipinfo、备份恢复

## 目标

实现运维诊断、出口 IP 检查和新格式备份恢复。

## 范围

- `doctor`
- `ipinfo`
- `export`
- `import`
- `sboxsub config show --show-secrets`
- `traffic check health`
- `sboxsub doctor`

## 技术方案

doctor 按模块输出 OK/ISSUE：

- 目录和权限。
- 二进制存在和版本。
- sing-box check。
- systemd/launchd 文件。
- 端口监听。
- API 可用性。
- traffic DB 和 timer。
- sub inputs 和服务。

ipinfo 通过实例本地 socks/http listener 发起请求，不使用 sing-box 管理 API。

agent 备份包使用 `backup_manifest.json`，只包含 `config.yaml` 和 `instances/*.yaml`，不包含 runtime manifest、traffic DB、downloads、logs、服务文件或 sboxsub inputs。

## 验收标准

- doctor 发现 ISSUE 时返回非零。
- ipinfo 支持 ipv4、ipv6、all。
- export 不包含 runtime/generated、traffic DB、downloads、logs。
- import 校验 `backup_manifest.json` 和 hash 后写入。
- export/import 拒绝订阅 bundle 和旧 native backup。
- 默认输出不展示敏感字段。

## 风险

- ipinfo 依赖外部 HTTP 服务。需要支持多个 endpoint fallback 和 timeout。
