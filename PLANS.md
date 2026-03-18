# 当前任务

## 260318 批量汇总吞掉提交详情

- 状态: 已完成修复与验证
- 分支: `bugfix/260318-ci-monitor-批量汇总吞掉提交详情`
- 问题描述:
  新版本通知引入批量合并后，同一 `message_group + atMobiles` 窗口内如果累积了多条事件，`commit_change` 原本的完整通知会被替换成单行摘要，导致用户感知为“消息丢失”。
- 已确认事实:
  `fetcher/fetch.go` 中 `buildSingleMessage` 仍然保留完整的提交消息模板。
  `fetcher/fetch.go` 中 `buildBatchMessage` 在 `len(items) > 1` 时只输出 `buildItemSummary` 的摘要行。
  webhook 批次按 `message_group + atMobiles` 合并，并带有防抖窗口。
- 复现路径:
  1. 同一通知分组在防抖窗口内进入两条及以上事件。
  2. 调用 `buildBatchMessage(items)`。
  3. 返回内容变成“已合并 N 条 CI 事件”加摘要列表，单条提交详情不再出现。
- 修复目标:
  批量通知仍保留“合并发送”能力，但不能吞掉 `commit_change` 的提交详情，至少要让合并消息中可见完整的单条事件内容。
- 最小修复方案:
  调整批量消息拼装逻辑，在合并通知中输出每条事件的完整消息正文，而不是仅输出摘要。
  保留批量通知头部和总量控制，避免扩大到抓取、状态判断或 webhook 发送协议。
- 实际修复:
  `fetcher/fetch.go` 的 `buildBatchMessage` 改为在多条事件场景输出逐条详细正文。
  新增 `buildBatchItemDetail`，直接复用 `buildSingleMessage`，只移除重复的 `[CI Bot]` 头。
  保留 15 条批量上限和“其余请查看看板/API”的截断逻辑。
- 验证计划:
  增加针对 `buildBatchMessage` 的回归测试，覆盖单条与多条 `commit_change`/`pipeline_change` 合并场景。
  运行 `go test ./fetcher`。
- 实际验证:
  `go test ./fetcher` 通过。
  `go test ./...` 通过。
- Review 1 结论:
  正确性与范围审查通过。修复仅影响批量消息正文生成，不改变 webhook 发送、分组、防抖、状态判断和单条通知路径。
- Review 2 结论:
  回归风险审查通过。批量消息长度相比摘要增加，但条数上限仍是 15，且仍保留超出条数的截断提示，风险可控。
- 风险与关注点:
  合并消息会比摘要更长，需要继续保留条数上限，避免超长消息。
