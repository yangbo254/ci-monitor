# 260318 批量汇总吞掉提交详情 代码片段记录

## 关注位置

- `fetcher/fetch.go`
  - `buildBatchMessage`
  - `buildSingleMessage`
  - `buildItemSummary`

## 现状摘要

- `buildSingleMessage` 负责输出单条事件完整正文。
- `buildBatchMessage` 在批量场景只拼接 `buildItemSummary`，因此提交详情被摘要覆盖。

## 预计修改点

- 让批量消息改为复用单条消息正文，并保留合并头和条数限制。
- 为批量场景补测试，固定住这次回归。

## 实际修改片段说明

- `fetcher/fetch.go`
  - `buildBatchMessage` 从 `buildItemSummary` 改为 `buildBatchItemDetail`。
  - `buildBatchItemDetail` 复用 `buildSingleMessage`，并去掉重复的 `[CI Bot]` 头部。
- `fetcher/fetch_test.go`
  - `TestBuildBatchMessageCommitChangePreservesDetails` 验证多条合并后仍包含完整 commit 详情。
  - `TestBuildBatchMessageSingleItemKeepsOriginalFormat` 验证单条路径输出未被修改。
