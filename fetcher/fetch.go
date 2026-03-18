package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ci-monitor/logger"
	ci "ci-monitor/types"

	"github.com/go-resty/resty/v2"
)

const (
	defaultNotifyDebounceSeconds = 20
	defaultWebhookTimeoutSeconds = 10
	maxWebhookBatchItems         = 25
	maxWebhookEventHistory       = 500
	maxWebhookBodyLogLength      = 600
)

var (
	previousStatus   = make(map[int]ci.ProjectStatus)
	previousStatusMu sync.RWMutex

	webhookEvents   = make([]ci.WebhookEvent, 0, maxWebhookEventHistory)
	webhookEventsMu sync.RWMutex
	webhookSeq      uint64

	notificationBatches   = make(map[string]*webhookBatch)
	notificationBatchesMu sync.Mutex
)

type pendingWebhookNotification struct {
	EventID      string
	ProjectID    int
	ProjectName  string
	EventType    string
	MessageGroup string
	AtMobiles    []string
	Intro        string
	Branch       string
	CommitSHA    string
	CommitAuthor string
	CommitTime   string
	CommitMsg    string
	CIStatus     string
	ReleaseCI    string
}

type webhookBatch struct {
	Key          string
	MessageGroup string
	AtMobiles    []string
	Items        []pendingWebhookNotification
	Timer        *time.Timer
	Timeout      time.Duration
}

// LoadConfig 读取配置文件
func LoadConfig(path string) (*ci.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ci.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	if !jsonHasKey(data, "notify_commit_change") {
		cfg.NotifyCommitChange = true
	}
	return &cfg, nil
}

func GetWebhookEvents(limit int) []ci.WebhookEvent {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	webhookEventsMu.RLock()
	defer webhookEventsMu.RUnlock()

	size := len(webhookEvents)
	if size == 0 {
		return []ci.WebhookEvent{}
	}

	if limit > size {
		limit = size
	}

	result := make([]ci.WebhookEvent, 0, limit)
	for i := size - 1; i >= size-limit; i-- {
		result = append(result, webhookEvents[i])
	}
	return result
}

// FetchAll 并发抓取所有项目状态
func FetchAll(cfg *ci.Config) []ci.ProjectStatus {
	var wg sync.WaitGroup
	ch := make(chan ci.ProjectStatus, len(cfg.Projects))

	for _, p := range cfg.Projects {
		wg.Add(1)
		go func(pc ci.ProjectConfig, cfg *ci.Config) {
			defer wg.Done()
			ps := FetchOne(pc, cfg)
			ch <- ps
		}(p, cfg)
	}

	wg.Wait()
	close(ch)

	list := make([]ci.ProjectStatus, 0, len(cfg.Projects))
	for ps := range ch {
		list = append(list, ps)
	}

	// 按 ProjectID 排序
	for i := 0; i < len(list)-1; i++ {
		for j := i + 1; j < len(list); j++ {
			if list[i].ProjectID > list[j].ProjectID {
				list[i], list[j] = list[j], list[i]
			}
		}
	}

	return list
}

// FetchOne 抓取单个项目状态
func FetchOne(c ci.ProjectConfig, cfg *ci.Config) ci.ProjectStatus {
	result := ci.ProjectStatus{
		ProjectID:   c.ID,
		ProjectName: c.Name,
		GroupID:     c.GroupID,
		Intro:       c.Intro,
		Branch:      c.Branch,
	}

	// --- Dev 分支 commit + pipeline ---
	client := resty.New().SetHeader("PRIVATE-TOKEN", c.Token)
	var commits []map[string]interface{}
	resp, err := client.R().
		SetQueryParam("ref_name", c.Branch).
		SetResult(&commits).
		Get(fmt.Sprintf("%s/api/v4/projects/%d/repository/commits", c.GitlabHost, c.ID))

	if err != nil || resp.IsError() {
		logger.Error.Printf("[%s] Dev commits fetch failed, err=%v status=%s", c.Name, err, httpStatus(resp))
	} else if len(commits) > 0 {
		sha := fmt.Sprintf("%v", commits[0]["id"])
		result.CommitSHA = sha
		result.CommitShortSHA = shortSHA(sha)
		result.CommitAuthor = fmt.Sprintf("%v", commits[0]["author_name"])
		result.CommitTime = fmt.Sprintf("%v", commits[0]["created_at"])
		result.CommitMessage = fmt.Sprintf("%v", commits[0]["title"])

		var pipelines []ci.PipelineInfo
		pResp, pErr := client.R().
			SetResult(&pipelines).
			Get(fmt.Sprintf("%s/api/v4/projects/%d/pipelines?ref=%s&per_page=1", c.GitlabHost, c.ID, c.Branch))
		if pErr != nil || (pResp != nil && pResp.IsError()) {
			logger.Error.Printf("[%s] Dev pipeline fetch failed, err=%v status=%s", c.Name, pErr, httpStatus(pResp))
		}
		if len(pipelines) > 0 {
			result.CI = pipelines[0]
		}
	}

	// --- Release 分支 commit + pipeline ---
	rc := resty.New().SetHeader("PRIVATE-TOKEN", c.ReleaseToken)
	var rcommits []map[string]interface{}
	resp2, err := rc.R().
		SetQueryParam("ref_name", c.ReleaseBranch).
		SetResult(&rcommits).
		Get(fmt.Sprintf("%s/api/v4/projects/%d/repository/commits", c.ReleaseHost, c.ReleaseID))

	if err != nil || resp2.IsError() {
		logger.Error.Printf("[%s] Release commits fetch failed, err=%v status=%s", c.Name, err, httpStatus(resp2))
	} else if len(rcommits) > 0 {
		sha := fmt.Sprintf("%v", rcommits[0]["id"])
		result.ReleaseSHA = sha
		result.ReleaseShortSHA = shortSHA(sha)
		result.ReleaseAuthor = fmt.Sprintf("%v", rcommits[0]["author_name"])
		result.ReleaseTime = fmt.Sprintf("%v", rcommits[0]["created_at"])
		result.ReleaseMessage = fmt.Sprintf("%v", rcommits[0]["title"])

		var pipelines []ci.PipelineInfo
		pResp, pErr := rc.R().
			SetResult(&pipelines).
			Get(fmt.Sprintf("%s/api/v4/projects/%d/pipelines?ref=%s&per_page=1", c.ReleaseHost, c.ReleaseID, c.ReleaseBranch))
		if pErr != nil || (pResp != nil && pResp.IsError()) {
			logger.Error.Printf("[%s] Release pipeline fetch failed, err=%v status=%s", c.Name, pErr, httpStatus(pResp))
		}
		if len(pipelines) > 0 {
			result.ReleaseCI = pipelines[0]
		}
	}

	result.StatusColor = PickColor(result)

	// --- 检测变化触发 webhook ---
	prev, exists := getPreviousStatus(c.ID)
	if exists {
		if cfg.NotifyCommitChange && prev.CommitSHA != result.CommitSHA {
			triggerWebhook(cfg, c, result, "commit_change")
		}
		if shouldNotifyPipelineChange(prev, result) {
			triggerWebhook(cfg, c, result, "pipeline_change")
		}
	}
	setPreviousStatus(c.ID, result)

	return result
}

// PickColor 计算项目状态颜色
func PickColor(r ci.ProjectStatus) string {
	if r.CI.Status == "failed" || r.ReleaseCI.Status == "failed" {
		return "red"
	}
	if r.CI.Status == "pending" || r.CI.Status == "running" ||
		r.ReleaseCI.Status == "pending" || r.ReleaseCI.Status == "running" {
		return "yellow"
	}
	if r.CI.Status == "success" && r.ReleaseCI.Status == "success" {
		return "green"
	}
	return "yellow"
}

// triggerWebhook 触发 webhook，并处理 @手机号
func triggerWebhook(cfg *ci.Config, c ci.ProjectConfig, ps ci.ProjectStatus, eventType string) {
	if strings.TrimSpace(c.MessageGroup) == "" {
		recordWebhookEvent(ci.WebhookEvent{
			EventID:      nextWebhookEventID(),
			ProjectID:    ps.ProjectID,
			ProjectName:  ps.ProjectName,
			EventType:    eventType,
			Stage:        "skipped",
			Timestamp:    time.Now().Format(time.RFC3339),
			MessageGroup: "",
			Detail:       "message_group 未配置，跳过发送",
		})
		return
	}

	// 解析 message_at，查找对应手机号
	atNames := parseCSV(c.MessageAt)
	atMobiles := make([]string, 0, len(atNames))
	for _, name := range atNames {
		name = strings.TrimSpace(name)
		for _, pb := range cfg.PhoneBooks {
			if pb.Name == name {
				atMobiles = append(atMobiles, pb.Phone)
			}
		}
	}

	enqueueWebhook(cfg, pendingWebhookNotification{
		EventID:      nextWebhookEventID(),
		ProjectID:    ps.ProjectID,
		ProjectName:  ps.ProjectName,
		EventType:    eventType,
		MessageGroup: c.MessageGroup,
		AtMobiles:    append([]string(nil), atMobiles...),
		Intro:        ps.Intro,
		Branch:       ps.Branch,
		CommitSHA:    ps.CommitShortSHA,
		CommitAuthor: ps.CommitAuthor,
		CommitTime:   ps.CommitTime,
		CommitMsg:    ps.CommitMessage,
		CIStatus:     ps.CI.Status,
		ReleaseCI:    ps.ReleaseCI.Status,
	})
}

// shortSHA 获取 SHA 前8位
func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func enqueueWebhook(cfg *ci.Config, n pendingWebhookNotification) {
	recordWebhookEvent(ci.WebhookEvent{
		EventID:      n.EventID,
		ProjectID:    n.ProjectID,
		ProjectName:  n.ProjectName,
		EventType:    n.EventType,
		Stage:        "queued",
		Timestamp:    time.Now().Format(time.RFC3339),
		MessageGroup: n.MessageGroup,
		AtMobiles:    append([]string(nil), n.AtMobiles...),
		Detail:       buildItemSummary(n),
	})

	key := buildBatchKey(n.MessageGroup, n.AtMobiles)
	timeout := webhookTimeout(cfg.WebhookTimeoutSeconds)

	notificationBatchesMu.Lock()
	batch, ok := notificationBatches[key]
	if !ok {
		batch = &webhookBatch{
			Key:          key,
			MessageGroup: n.MessageGroup,
			AtMobiles:    append([]string(nil), n.AtMobiles...),
			Timeout:      timeout,
		}
		notificationBatches[key] = batch
	}
	batch.Items = append(batch.Items, n)
	size := len(batch.Items)
	if batch.Timer == nil {
		wait := debounceDuration(cfg.NotifyDebounceSeconds)
		batch.Timer = time.AfterFunc(wait, func() {
			flushWebhookBatch(key)
		})
	}
	notificationBatchesMu.Unlock()

	logger.Info.Printf("[webhook] queued id=%s project=%s event=%s batch_size=%d", n.EventID, n.ProjectName, n.EventType, size)

	if size >= maxWebhookBatchItems {
		go flushWebhookBatch(key)
	}
}

func flushWebhookBatch(key string) {
	notificationBatchesMu.Lock()
	batch, ok := notificationBatches[key]
	if !ok {
		notificationBatchesMu.Unlock()
		return
	}
	delete(notificationBatches, key)
	notificationBatchesMu.Unlock()

	content := buildBatchMessage(batch.Items)
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
		"at": map[string]interface{}{
			"isAtAll":   false,
			"atMobiles": batch.AtMobiles,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		markBatchAsFailed(batch, fmt.Sprintf("payload 编码失败: %v", err), "")
		return
	}

	urlWithTimestamp := fmt.Sprintf("%s&timestamp=%d", batch.MessageGroup, time.Now().UnixMilli())
	client := &http.Client{Timeout: batch.Timeout}
	resp, err := client.Post(urlWithTimestamp, "application/json", bytes.NewReader(data))
	if err != nil {
		markBatchAsFailed(batch, fmt.Sprintf("请求失败: %v", err), "")
		return
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	body := strings.TrimSpace(string(bodyBytes))
	if readErr != nil {
		markBatchAsFailed(batch, fmt.Sprintf("读取响应失败: %v", readErr), resp.Status)
		return
	}

	logger.Info.Printf("[webhook] sent projects=%d status=%s body=%s", len(batch.Items), resp.Status, truncate(body, maxWebhookBodyLogLength))
	markBatchAsSuccess(batch, resp.Status, body)
}

func markBatchAsFailed(batch *webhookBatch, reason string, httpStatus string) {
	logger.Error.Printf("[webhook] failed projects=%d reason=%s", len(batch.Items), reason)
	for _, item := range batch.Items {
		recordWebhookEvent(ci.WebhookEvent{
			EventID:      item.EventID,
			ProjectID:    item.ProjectID,
			ProjectName:  item.ProjectName,
			EventType:    item.EventType,
			Stage:        "failed",
			Timestamp:    time.Now().Format(time.RFC3339),
			MessageGroup: item.MessageGroup,
			AtMobiles:    append([]string(nil), item.AtMobiles...),
			Detail:       reason,
			HTTPStatus:   httpStatus,
		})
	}
}

func markBatchAsSuccess(batch *webhookBatch, httpStatus string, body string) {
	for _, item := range batch.Items {
		recordWebhookEvent(ci.WebhookEvent{
			EventID:      item.EventID,
			ProjectID:    item.ProjectID,
			ProjectName:  item.ProjectName,
			EventType:    item.EventType,
			Stage:        "success",
			Timestamp:    time.Now().Format(time.RFC3339),
			MessageGroup: item.MessageGroup,
			AtMobiles:    append([]string(nil), item.AtMobiles...),
			Detail:       truncate(body, maxWebhookBodyLogLength),
			HTTPStatus:   httpStatus,
		})
	}
}

func buildBatchMessage(items []pendingWebhookNotification) string {
	if len(items) == 0 {
		return "[CI Bot] 空通知，忽略"
	}

	if len(items) == 1 {
		return buildSingleMessage(items[0])
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("[CI Bot]\n已合并 %d 条 CI 事件，避免通知风暴：\n", len(items)))

	for i, item := range items {
		if i >= 15 {
			b.WriteString(fmt.Sprintf("... 其余 %d 条请查看看板/API\n", len(items)-15))
			break
		}

		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("%d)\n%s\n", i+1, buildBatchItemDetail(item)))
	}
	return b.String()
}

func buildSingleMessage(item pendingWebhookNotification) string {
	switch item.EventType {
	case "commit_change":
		return fmt.Sprintf(
			"[CI Bot]\n项目[%s]有新的提交\n简介: %s\nBranch: %s\nCommit SHA: %s\n作者: %s\n提交时间: %s\n提交信息: %s\nCommit CI 状态: %s",
			item.ProjectName,
			item.Intro,
			item.Branch,
			item.CommitSHA,
			item.CommitAuthor,
			item.CommitTime,
			item.CommitMsg,
			item.CIStatus,
		)
	case "pipeline_change":
		return fmt.Sprintf(
			"[CI Bot]\n项目[%s]流水线状态变化\nCommit CI: %s\nRelease CI: %s",
			item.ProjectName,
			item.CIStatus,
			item.ReleaseCI,
		)
	default:
		return fmt.Sprintf("[CI Bot]\n项目[%s]发生事件: %s", item.ProjectName, item.EventType)
	}
}

func buildItemSummary(item pendingWebhookNotification) string {
	switch item.EventType {
	case "commit_change":
		return fmt.Sprintf("[%s] commit=%s author=%s ci=%s", item.ProjectName, item.CommitSHA, item.CommitAuthor, item.CIStatus)
	case "pipeline_change":
		return fmt.Sprintf("[%s] ci=%s release=%s", item.ProjectName, item.CIStatus, item.ReleaseCI)
	default:
		return fmt.Sprintf("[%s] event=%s", item.ProjectName, item.EventType)
	}
}

func buildBatchItemDetail(item pendingWebhookNotification) string {
	return strings.TrimPrefix(buildSingleMessage(item), "[CI Bot]\n")
}

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func buildBatchKey(messageGroup string, mobiles []string) string {
	keys := append([]string(nil), mobiles...)
	sort.Strings(keys)
	return fmt.Sprintf("%s|%s", messageGroup, strings.Join(keys, ","))
}

func debounceDuration(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultNotifyDebounceSeconds
	}
	return time.Duration(seconds) * time.Second
}

func webhookTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = defaultWebhookTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

func shouldNotifyPipelineChange(prev ci.ProjectStatus, current ci.ProjectStatus) bool {
	changed := prev.CI.Status != current.CI.Status || prev.ReleaseCI.Status != current.ReleaseCI.Status
	if !changed {
		return false
	}

	currentColor := PickColor(current)
	if currentColor == "yellow" {
		return false
	}
	return PickColor(prev) != currentColor
}

func getPreviousStatus(id int) (ci.ProjectStatus, bool) {
	previousStatusMu.RLock()
	defer previousStatusMu.RUnlock()
	ps, ok := previousStatus[id]
	return ps, ok
}

func setPreviousStatus(id int, status ci.ProjectStatus) {
	previousStatusMu.Lock()
	defer previousStatusMu.Unlock()
	previousStatus[id] = status
}

func SeedPreviousStatus(list []ci.ProjectStatus) int {
	previousStatusMu.Lock()
	defer previousStatusMu.Unlock()

	seeded := 0
	for _, status := range list {
		if status.ProjectID == 0 {
			continue
		}
		previousStatus[status.ProjectID] = status
		seeded++
	}
	return seeded
}

func jsonHasKey(data []byte, key string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, ok := raw[key]
	return ok
}

func recordWebhookEvent(event ci.WebhookEvent) {
	webhookEventsMu.Lock()
	defer webhookEventsMu.Unlock()

	webhookEvents = append(webhookEvents, event)
	if len(webhookEvents) > maxWebhookEventHistory {
		webhookEvents = webhookEvents[len(webhookEvents)-maxWebhookEventHistory:]
	}
}

func nextWebhookEventID() string {
	seq := atomic.AddUint64(&webhookSeq, 1)
	return fmt.Sprintf("wh-%d-%d", time.Now().UnixMilli(), seq)
}

func httpStatus(resp *resty.Response) string {
	if resp == nil {
		return "n/a"
	}
	return resp.Status()
}

func truncate(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}
