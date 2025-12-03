package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"ci-monitor/logger"
	"ci-monitor/storage"
)

var tpl = `
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8"/>
<title>云机房 CI 状态看板</title>
<style>
body { font-family: "Segoe UI", Tahoma, Geneva, Verdana, sans-serif; background: #f5f5f5; }
h2 { text-align: center; margin-top: 20px; }
table { border-collapse: collapse; width: 95%; margin: 20px auto; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
th, td { padding: 10px 15px; text-align: center; border: 1px solid #ccc; }
th { background-color: #4CAF50; color: white; }
tr.red { background: #ffb3b3; }
tr.yellow { background: #f1d549ff; }
tr.green { background: #b3ffb3; }
tr:hover { background-color: #e0f7fa; }
td.sha, td.id { font-family: monospace; }
td.time { font-size: 0.9em; color: #333; }
td.bold { font-weight: bold; }
.group-header { background: #ddd; font-weight: bold; }
.commit-msg { font-size: 0.85em; color: #555; }
</style>
</head>

<body>
<h2>CI 状态看板</h2>

<div id="content">加载中...</div>

<script>
// 每次请求 /api/status，重新构建表格
function refreshData() {
    fetch('/api/status')
        .then(resp => resp.json())
        .then(data => {
            let html = '<table>';
            html += ` + "`" + `
<tr>
  <th>ID</th>
  <th>项目</th>
  <th>Intro</th>
  <th>Branch</th>
  <th>Commit SHA</th>
  <th>Commit Message</th>
  <th>提交人</th>
  <th>提交时间</th>
  <th>Commit CI</th>
  <th>Release SHA</th>
  <th>提交人</th>
  <th>提交时间</th>
  <th>Release CI</th>
</tr>` + "`" + `;

            for (let group in data) {
                html += ` + "`" + `<tr class="group-header"><td colspan="13">${group}</td></tr>` + "`" + `;
                data[group].forEach((p, index) => {
                    html += ` + "`" + `
<tr class="${p.StatusColor}">
  <td class="id">${index + 1}</td>
  <td class="bold">${p.ProjectName}</td>
  <td>${p.Intro}</td>
  <td>${p.Branch}</td>
  <td class="sha" title="${p.CommitSHA}">${p.CommitShortSHA}</td>
  <td class="commit-msg" title="${p.CommitMessage}">${p.CommitShortMsg}</td>
  <td>${p.CommitAuthor}</td>
  <td class="time">${p.CommitTimeFmt}</td>
  <td class="bold">${p.CI.status}</td>

  <td class="sha" title="${p.ReleaseSHA}">${p.ReleaseShortSHA}</td>
  <td>${p.ReleaseAuthor}</td>
  <td class="time">${p.ReleaseTimeFmt}</td>
  <td class="bold">${p.ReleaseCI.status}</td>
</tr>` + "`" + `;
                });
            }
            html += '</table>';
            document.getElementById("content").innerHTML = html;
        })
        .catch(err => {
            document.getElementById("content").innerHTML = "加载失败: " + err;
        });
}

// 每 5 秒刷新
setInterval(refreshData, 4000);

// 首次加载
refreshData();

</script>
</body>
</html>
`

// formatTime 格式化 UTC 到北京时间
func formatTime(s string) string {
	if s == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return t.In(loc).Format("2006-01-02 15:04:05")
}

// StartHTTP 启动前端页面 + 后端 API
func StartHTTP() {
	// 前端页面
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t, err := template.New("page").Parse(tpl)
		if err != nil {
			logger.Error.Printf("模板解析失败: %v", err)
			w.WriteHeader(500)
			return
		}
		t.Execute(w, nil)
	})

	// 后端 API
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		logger.Debug.Printf("API访问: %s %s from %s", r.Method, r.RequestURI, r.RemoteAddr)

		grouped, _ := storage.LoadGroupedProjectStatusWithoutGreen()
		groupedN, _ := storage.LoadGroupedProjectStatus()

		for key, value := range groupedN {
			grouped[key] = value
		}

		// 对返回 JSON 做时间格式转换、截短 SHA
		resp := map[string]interface{}{}
		for g, arr := range grouped {
			rows := []map[string]interface{}{}
			for _, p := range arr {
				rows = append(rows, map[string]interface{}{
					"ProjectID":      p.ProjectID,
					"ProjectName":    p.ProjectName,
					"Intro":          p.Intro,
					"Branch":         p.Branch,
					"StatusColor":    p.StatusColor,
					"CommitSHA":      p.CommitSHA,
					"CommitShortSHA": shortSHA(p.CommitSHA),
					"CommitAuthor":   p.CommitAuthor,
					"CommitTimeFmt":  formatTime(p.CommitTime),
					"CommitMessage":  p.CommitMessage,
					"CommitShortMsg": shortMessage(p.CommitMessage),
					"CI":             p.CI,

					"ReleaseSHA":      p.ReleaseSHA,
					"ReleaseShortSHA": shortSHA(p.ReleaseSHA),
					"ReleaseAuthor":   p.ReleaseAuthor,
					"ReleaseTimeFmt":  formatTime(p.ReleaseTime),
					"ReleaseCI":       p.ReleaseCI,
				})
			}
			resp[g] = rows
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	fmt.Println("访问地址: http://0.0.0.0:8080/")
	http.ListenAndServe(":8080", nil)
}

// shortSHA 取前 8 位
func shortSHA(s string) string {
	if len(s) < 8 {
		return s
	}
	return s[:8]
}

// shortMessage 返回简短 commit message，按字符截断，不破坏中文
func shortMessage(msg string) string {
	runes := []rune(msg) // 将字符串转成 rune 切片
	if len(runes) > 14 {
		return string(runes[:14]) + "..."
	}
	return msg
}
