package fetcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
	"strings"

	"ci-monitor/logger"
	ci "ci-monitor/types"
	"github.com/go-resty/resty/v2"
)

var previousStatus = make(map[int]ci.ProjectStatus) // 内存缓存上一次状态

// LoadConfig 读取配置文件
func LoadConfig(path string) (*ci.Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ci.Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// FetchAll 并发抓取所有项目状态
func FetchAll(cfg *ci.Config) []ci.ProjectStatus {
	var wg sync.WaitGroup
	ch := make(chan ci.ProjectStatus, len(cfg.Projects))

	for _, p := range cfg.Projects {
		wg.Add(1)
		go func(pc ci.ProjectConfig,cfg *ci.Config) {
			defer wg.Done()
			ps := FetchOne(pc,cfg)
			ch <- ps
		}(p,cfg)
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
	logger.Debug.Printf("[%s] Fetch dev commits from %s", c.Name, c.GitlabHost)
	resp, err := client.R().
		SetQueryParam("ref_name", c.Branch).
		SetResult(&commits).
		Get(fmt.Sprintf("%s/api/v4/projects/%d/repository/commits", c.GitlabHost, c.ID))

	if err != nil || resp.IsError() {
		logger.Error.Printf("[%s] Dev commits fetch failed: %v", c.Name, err)
	} else if len(commits) > 0 {
		sha := fmt.Sprintf("%v", commits[0]["id"])
		result.CommitSHA = sha
		result.CommitShortSHA = shortSHA(sha)
		result.CommitAuthor = fmt.Sprintf("%v", commits[0]["author_name"])
		result.CommitTime = fmt.Sprintf("%v", commits[0]["created_at"])
		result.CommitMessage = fmt.Sprintf("%v", commits[0]["title"])
		logger.Info.Printf("[%s] Dev commit: %s by %s at %s msg:%s", c.Name, result.CommitSHA, result.CommitAuthor, result.CommitTime, result.CommitMessage)

		var pipelines []ci.PipelineInfo
		client.R().
			SetResult(&pipelines).
			Get(fmt.Sprintf("%s/api/v4/projects/%d/pipelines?ref=%s&per_page=1", c.GitlabHost, c.ID, c.Branch))
		if len(pipelines) > 0 {
			result.CI = pipelines[0]
			logger.Info.Printf("[%s] Dev latest pipeline status: %s", c.Name, result.CI.Status)
		}
	}

	// --- Release 分支 commit + pipeline ---
	rc := resty.New().SetHeader("PRIVATE-TOKEN", c.ReleaseToken)
	var rcommits []map[string]interface{}
	logger.Debug.Printf("[%s] Fetch release commits from %s", c.Name, c.ReleaseHost)
	resp2, err := rc.R().
		SetQueryParam("ref_name", c.ReleaseBranch).
		SetResult(&rcommits).
		Get(fmt.Sprintf("%s/api/v4/projects/%d/repository/commits", c.ReleaseHost, c.ReleaseID))

	if err != nil || resp2.IsError() {
		logger.Error.Printf("[%s] Release commits fetch failed: %v", c.Name, err)
	} else if len(rcommits) > 0 {
		sha := fmt.Sprintf("%v", rcommits[0]["id"])
		result.ReleaseSHA = sha
		result.ReleaseShortSHA = shortSHA(sha)
		result.ReleaseAuthor = fmt.Sprintf("%v", rcommits[0]["author_name"])
		result.ReleaseTime = fmt.Sprintf("%v", rcommits[0]["created_at"])
		result.ReleaseMessage = fmt.Sprintf("%v", rcommits[0]["title"])
		logger.Info.Printf("[%s] Release commit: %s by %s at %s msg:%s", c.Name, result.ReleaseSHA, result.ReleaseAuthor, result.ReleaseTime, result.ReleaseMessage)

		var pipelines []ci.PipelineInfo
		rc.R().
			SetResult(&pipelines).
			Get(fmt.Sprintf("%s/api/v4/projects/%d/pipelines?ref=%s&per_page=1", c.ReleaseHost, c.ReleaseID, c.ReleaseBranch))
		if len(pipelines) > 0 {
			result.ReleaseCI = pipelines[0]
			logger.Info.Printf("[%s] Release latest pipeline status: %s", c.Name, result.ReleaseCI.Status)
		}
	}

	result.StatusColor = PickColor(result)

	// --- 检测变化触发 webhook ---
	prev, exists := previousStatus[c.ID]
	if exists {
		if prev.CommitSHA != result.CommitSHA {
			triggerWebhook(cfg, c, result, "commit_change")
		}
		if prev.CI.Status != result.CI.Status || prev.ReleaseCI.Status != result.ReleaseCI.Status {
			triggerWebhook(cfg, c, result, "pipeline_change")
		}
	}
	previousStatus[c.ID] = result

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
	if c.MessageGroup == "" {
		return
	}

	// 解析 message_at，查找对应手机号
	atNames := strings.Split(c.MessageAt, ",")
	atMobiles := []string{}
	for _, name := range atNames {
		name = strings.TrimSpace(name)
		for _, pb := range cfg.PhoneBooks {
			if pb.Name == name {
				atMobiles = append(atMobiles, pb.Phone)
			}
		}
	}

	var msg string
	if len(atNames) > 0 {
		msg = "[CI Bot]"
	}
	switch eventType {
	case "commit_change":
		msg += fmt.Sprintf(
			"项目 [%s] 有新的提交\n简介: %s\nBranch: %s\nCommit SHA: %s\n作者: %s\n提交时间: %s\n提交信息: %s\nCommit CI 状态: %s\nRelease SHA: %s\n作者: %s\n提交时间: %s\n提交信息: %s\nRelease CI 状态: %s",
			ps.ProjectName,
			ps.Intro,
			ps.Branch,
			ps.CommitShortSHA,
			ps.CommitAuthor,
			ps.CommitTime,
			ps.CommitMessage,
			ps.CI.Status,
			ps.ReleaseShortSHA,
			ps.ReleaseAuthor,
			ps.ReleaseTime,
			ps.ReleaseMessage,
			ps.ReleaseCI.Status,
		)
	case "pipeline_change":
		msg += fmt.Sprintf(
			"项目 [%s] 的流水线状态发生变化\nCommit CI 状态: %s\nRelease CI 状态: %s",
			ps.ProjectName,
			ps.CI.Status,
			ps.ReleaseCI.Status,
		)
	default:
		msg += fmt.Sprintf("项目 [%s] 有事件: %s", ps.ProjectName, eventType)
	}

	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": msg,
		},
		"at": map[string]interface{}{
			"isAtAll":   false,
			"atMobiles": atMobiles,
		},
	}

	data, _ := json.Marshal(payload)
	timestamp := time.Now().UnixMilli()
	urlWithTimestamp := fmt.Sprintf("%s&timestamp=%d", c.MessageGroup, timestamp)

	go func() {
		resp, err := http.Post(urlWithTimestamp, "application/json", bytes.NewReader(data))
		if err != nil {
			logger.Error.Printf("[%s] webhook触发失败: %v", ps.ProjectName, err)
			return
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logger.Error.Printf("[%s] webhook响应读取失败: %v", ps.ProjectName, err)
			return
		}

		logger.Info.Printf("[%s] webhook触发成功: %s 状态码 %s, 返回体: %s", ps.ProjectName, eventType, resp.Status, string(body))
	}()
}

// shortSHA 获取 SHA 前8位
func shortSHA(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}


