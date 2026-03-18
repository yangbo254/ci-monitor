package fetcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ci "ci-monitor/types"
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

func TestLoadConfigDefaultsNotifyCommitChangeToTrue(t *testing.T) {
	cfg := loadConfigForTest(t, `{"projects":[],"phone_books":[],"groupinfo":[]}`)
	if !cfg.NotifyCommitChange {
		t.Fatal("expected notify_commit_change to default to true when omitted")
	}
}

func TestLoadConfigKeepsExplicitNotifyCommitChangeFalse(t *testing.T) {
	cfg := loadConfigForTest(t, `{"notify_commit_change":false,"projects":[],"phone_books":[],"groupinfo":[]}`)
	if cfg.NotifyCommitChange {
		t.Fatal("expected explicit notify_commit_change=false to be preserved")
	}
}

func TestSeedPreviousStatusRestoresSnapshot(t *testing.T) {
	resetPreviousStatusForTest(t)

	seeded := SeedPreviousStatus([]ci.ProjectStatus{
		{ProjectID: 94, CommitSHA: "abc12345"},
	})
	if seeded != 1 {
		t.Fatalf("expected to seed 1 project status, got %d", seeded)
	}

	got, ok := getPreviousStatus(94)
	if !ok {
		t.Fatal("expected previous status to be restored for project 94")
	}
	if got.CommitSHA != "abc12345" {
		t.Fatalf("expected restored commit sha abc12345, got %q", got.CommitSHA)
	}
}

func loadConfigForTest(t *testing.T, body string) *ci.Config {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func resetPreviousStatusForTest(t *testing.T) {
	t.Helper()

	previousStatusMu.Lock()
	saved := previousStatus
	previousStatus = make(map[int]ci.ProjectStatus)
	previousStatusMu.Unlock()

	t.Cleanup(func() {
		previousStatusMu.Lock()
		previousStatus = saved
		previousStatusMu.Unlock()
	})
}
