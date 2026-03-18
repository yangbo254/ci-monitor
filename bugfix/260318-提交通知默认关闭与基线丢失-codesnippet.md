# 260318 提交通知默认关闭与基线丢失 代码片段记录

## 关注位置

- `fetcher/fetch.go`
  - `LoadConfig`
  - `getPreviousStatus`
  - `setPreviousStatus`
- `main.go`
  - 启动流程

## 现状摘要

- `notify_commit_change` 缺失时会落为 `false`。
- `previousStatus` 只存在进程内，未从存储恢复。

## 预计修改点

- 为 `notify_commit_change` 增加缺省为 `true` 的兼容逻辑。
- 在启动时读取已存储状态并灌入 `previousStatus`。
- 补回归测试固定配置默认值和基线恢复行为。

## 实际修改片段说明

- `fetcher/fetch.go`
  - `LoadConfig` 增加 `notify_commit_change` 缺省为 `true` 的兼容逻辑。
  - 新增 `SeedPreviousStatus` 和 `jsonHasKey`。
- `main.go`
  - 启动后读取已存储状态并恢复比较基线。
- `fetcher/fetch_test.go`
  - 增加配置默认值和基线恢复回归测试。
