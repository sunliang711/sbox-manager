# T03 sing-box 生成器与 Runtime

## 目标

从新领域模型生成稳定 sing-box JSON，并实现 Runtime Plan、manifest 和原子写入。

## 范围

- sing-box JSON 生成器。
- `render model`。
- `render sing-box INSTANCE`。
- `render sub`。
- `export-config clash|premium-clash|surge|sing-box USER`。
- `check` diff 预览。
- `start/restart` runtime apply。
- manifest 读写。

## 技术方案

生成器保持纯函数，不读写文件。Runtime 层负责写入和 diff。

生成顺序必须稳定：

1. log。
2. dns。
3. inbounds。
4. outbounds。
5. route。
6. services/experimental。

## 验收标准

- 相同输入生成字节级一致 JSON。
- `check` 只读。
- `start/restart` 原子写文件并更新 manifest，然后调用服务管理器。
- `start` no-change 时不改写 generated 或 manifest，但仍可调用 service manager start。
- `restart` no-change 时不改写 generated 或 manifest，但仍强制调用 service manager restart。
- 生成文件可通过 `sing-box check -c`。

## 风险

- sing-box 版本差异导致配置字段变化。需要在 generator 测试中固定支持版本范围。
