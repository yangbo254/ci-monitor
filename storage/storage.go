package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	

	ci "ci-monitor/types"
	"ci-monitor/logger"

	"github.com/redis/go-redis/v9"
)

var (
	redisClient *redis.Client
	useRedis    bool
	memStore    []ci.ProjectStatus
	lock        sync.RWMutex
	ctx         = context.Background()
)

func Init(redisAddr string) {
	if redisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr: redisAddr,
		})
		_, err := redisClient.Ping(ctx).Result()
		if err != nil {
			logger.Error.Printf("Redis连接失败，改为内存存储: %v", err)
			useRedis = false
		} else {
			useRedis = true
			logger.Info.Println("使用 Redis 存储 ProjectStatus")
		}
	} else {
		useRedis = false
		logger.Info.Println("未配置 Redis，使用内存存储 ProjectStatus")
	}
}

func SaveProjectStatus(list []ci.ProjectStatus) {
	if useRedis {
		for _, p := range list {
			key := fmt.Sprintf("project:%d", p.ProjectID)
			data, _ := json.Marshal(p)
			if err := redisClient.Set(ctx, key, data, 0).Err(); err != nil {
				logger.Error.Printf("Redis写入失败: %v", err)
			}
		}
	} else {
		lock.Lock()
		memStore = list
		lock.Unlock()
	}
}

func LoadProjectStatus() ([]ci.ProjectStatus, error) {
	if useRedis {
		keys, err := redisClient.Keys(ctx, "project:*").Result()
		if err != nil {
			return nil, err
		}
		list := make([]ci.ProjectStatus, 0)
		for _, k := range keys {
			val, err := redisClient.Get(ctx, k).Result()
			if err != nil {
				continue
			}
			var p ci.ProjectStatus
			if err := json.Unmarshal([]byte(val), &p); err == nil {
				list = append(list, p)
			}
		}
		return list, nil
	} else {
		lock.RLock()
		defer lock.RUnlock()
		return memStore, nil
	}
}

func LoadGroupedProjectStatus() (map[string][]ci.ProjectStatus, error) {
	cfg, err := LoadConfig("config.json")
	if err != nil {
		return nil, err
	}

	statusList, _ := LoadProjectStatus()

	groupMap := make(map[int]string)
	for _, g := range cfg.GroupInfo {
		groupMap[g.ID] = g.Name
	}

	grouped := make(map[string][]ci.ProjectStatus)
	for _, s := range statusList {
		grpName := groupMap[s.GroupID]
		if grpName == "" {
			grpName = "未分组"
		}
		grouped[grpName] = append(grouped[grpName], s)
	}

	// 按 ProjectID 排序每组
	for k := range grouped {
		list := grouped[k]
		for i := 0; i < len(list)-1; i++ {
			for j := i + 1; j < len(list); j++ {
				if list[i].ProjectID > list[j].ProjectID {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
		grouped[k] = list
	}

	return grouped, nil
}

func LoadConfig(path string) (*ci.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg ci.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
