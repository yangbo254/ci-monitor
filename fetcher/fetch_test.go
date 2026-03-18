package fetcher

import (
	"strings"
	"testing"
)

func TestBuildBatchMessageCommitChangePreservesDetails(t *testing.T) {
	items := []pendingWebhookNotification{
		{
			ProjectName:  "lwyview",
			EventType:    "commit_change",
			Intro:        "中心前端服务",
			Branch:       "dev",
			CommitSHA:    "e1836019",
			CommitAuthor: "zhangyong",
			CommitTime:   "2026-03-17T03:54:52.000Z",
			CommitMsg:    "new feature",
			CIStatus:     "pending",
		},
		{
			ProjectName: "lwyview",
			EventType:   "pipeline_change",
			CIStatus:    "running",
			ReleaseCI:   "success",
		},
	}

	got := buildBatchMessage(items)

	expectedParts := []string{
		"[CI Bot]\n已合并 2 条 CI 事件，避免通知风暴：",
		"项目[lwyview]有新的提交",
		"简介: 中心前端服务",
		"Branch: dev",
		"Commit SHA: e1836019",
		"作者: zhangyong",
		"提交时间: 2026-03-17T03:54:52.000Z",
		"提交信息: new feature",
		"Commit CI 状态: pending",
		"项目[lwyview]流水线状态变化",
		"Release CI: success",
	}

	for _, part := range expectedParts {
		if !strings.Contains(got, part) {
			t.Fatalf("batch message missing %q.\nmessage:\n%s", part, got)
		}
	}
}

func TestBuildBatchMessageSingleItemKeepsOriginalFormat(t *testing.T) {
	item := pendingWebhookNotification{
		ProjectName:  "lwyview",
		EventType:    "commit_change",
		Intro:        "中心前端服务",
		Branch:       "dev",
		CommitSHA:    "e1836019",
		CommitAuthor: "zhangyong",
		CommitTime:   "2026-03-17T03:54:52.000Z",
		CommitMsg:    "new feature",
		CIStatus:     "pending",
	}

	got := buildBatchMessage([]pendingWebhookNotification{item})
	want := buildSingleMessage(item)
	if got != want {
		t.Fatalf("single item batch message changed unexpectedly.\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
