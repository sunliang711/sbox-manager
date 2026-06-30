# sboxctl / sboxsub 协议端到端验证与订阅修复记录

## 问题背景

- 测试目标：在远程测试机 `root@10.2.13.230` 上验证 `sboxctl` 支持的全部 inbound/outbound 协议真实可通，并验证 `sboxctl sub export -> sboxsub import/start -> 订阅配置获取后可用`。
- 测试环境：Debian 13，sing-box `1.13.14`，本项目本地交叉编译 linux/amd64 后上传到 `/usr/local/bin/sboxctl` 和 `/usr/local/bin/sboxsub`。

## 根因分析

- sing-box 订阅输出中包含 outbound 不支持的 `udp` 字段，导致 `sing-box check` 直接失败。
- WebSocket 订阅 transport 会输出 runtime 生成器不会输出的 `host` 字段，sing-box 1.13 对该 outbound transport 字段报 unknown field。
- VLESS REALITY 从 inbound 导出订阅节点时缺少客户端必需的 uTLS 配置，sing-box 报 `uTLS is required by reality client`。

## 修复方案

- `internal/subscription/render.go`：移除 sing-box 订阅 outbound 的 `udp` 输出；WebSocket 不再输出 transport `host`，只保留 `headers.Host`。
- `internal/generator/singbox/subscription.go`：REALITY 订阅节点自动补充 `utls.enabled=true` 和 `fingerprint=chrome`。
- 补充单元测试覆盖 sing-box 订阅非法字段过滤和 REALITY 订阅 uTLS。

## 验证结果

- 本地验证：`go test ./...` 通过。
- 远程真实链路验证：`/opt/sbox-protocol-e2e-report-20260630214222.log`。
- 远程结果：`PASS=56 FAIL=0`。
- 覆盖范围：
  - inbound：VMess raw/ws/http/quic/grpc/httpupgrade、VLESS raw/ws/http/quic/grpc/httpupgrade、VLESS REALITY、AnyTLS、Shadowsocks、SOCKS5、HTTP。
  - outbound：direct、block、ref、Shadowsocks、VMess raw/ws/http/quic/grpc/httpupgrade、VLESS raw/ws/http/quic/grpc/httpupgrade、VLESS REALITY、AnyTLS、Trojan、Hysteria2、SOCKS5、HTTP。
  - 订阅链路：`sboxctl sub export in-all` 导出 bundle，`sboxsub import --replace-all` 导入，`sboxsub start` 启动后获取 `/sing-box/alice`，逐个订阅节点包装本地 inbound 后真实访问 echo 服务，全部通过。

## 风险与后续建议

- 本次改动只影响 sing-box 订阅输出和 REALITY 订阅节点补全，不改变 runtime 实例生成逻辑。
- 远程测试使用自签 CA 并加入测试机系统信任，仅用于 E2E 验证。
