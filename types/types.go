package types

type PipelineInfo struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

type ProjectStatus struct {
	ProjectID      int          `json:"id"`
	ProjectName    string       `json:"name"`
	GroupID        int          `json:"group_id"`
	Branch         string       `json:"branch"`
	Intro          string       `json:"intro"`
	CommitSHA      string       `json:"commit_sha"`
	CommitShortSHA string       `json:"commit_short_sha"`
	CommitAuthor   string       `json:"commit_author"`
	CommitTime     string       `json:"commit_time"`
	CommitMessage  string       `json:"commit_message"`
	CI             PipelineInfo `json:"ci"`

	ReleaseSHA      string       `json:"release_sha"`
	ReleaseShortSHA string       `json:"release_short_sha"`
	ReleaseAuthor   string       `json:"release_author"`
	ReleaseTime     string       `json:"release_time"`
	ReleaseMessage  string       `json:"release_message"`
	ReleaseCI       PipelineInfo `json:"release_ci"`

	StatusColor string `json:"status_color"`
}

type ProjectConfig struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Branch        string `json:"branch"`
	Token         string `json:"token"`
	GitlabHost    string `json:"gitlab_host"`
	ReleaseID     int    `json:"release_id"`
	ReleaseBranch string `json:"release_branch"`
	ReleaseToken  string `json:"release_token"`
	ReleaseHost   string `json:"release_host"`
	GroupID       int    `json:"group_id"`
	MessageGroup  string `json:"message_group"`
	MessageAt     string `json:"message_at"`
	Intro         string `json:"intro"`
}

type GroupInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type PhoneBook struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type Config struct {
	RedisAddr             string          `json:"redis_addr"`
	LogLevel              string          `json:"log_level"`
	NotifyCommitChange    bool            `json:"notify_commit_change"`
	NotifyDebounceSeconds int             `json:"notify_debounce_seconds"`
	WebhookTimeoutSeconds int             `json:"webhook_timeout_seconds"`
	PhoneBooks            []PhoneBook     `json:"phone_books"`
	GroupInfo             []GroupInfo     `json:"groupinfo"`
	Projects              []ProjectConfig `json:"projects"`
}

type WebhookEvent struct {
	EventID      string   `json:"event_id"`
	ProjectID    int      `json:"project_id"`
	ProjectName  string   `json:"project_name"`
	EventType    string   `json:"event_type"`
	Stage        string   `json:"stage"`
	Timestamp    string   `json:"timestamp"`
	MessageGroup string   `json:"message_group"`
	AtMobiles    []string `json:"at_mobiles"`
	Detail       string   `json:"detail,omitempty"`
	HTTPStatus   string   `json:"http_status,omitempty"`
}
