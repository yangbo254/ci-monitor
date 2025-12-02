package main

import (
	"ci-monitor/fetcher"
	"ci-monitor/logger"
	"ci-monitor/storage"
	"ci-monitor/web"
	"time"
	"log"
)

func main() {
	logger.Init("ci-monitor.log")
	log.Println("启动 CI 监控程序...")

	// 初始化存储, 为空表示使用内存
	storage.Init("")

	// 定时抓取
	go func() {
		for {
			cfg, err := fetcher.LoadConfig("config.json")
			if err != nil {
				log.Println("读取配置失败:", err)
				time.Sleep(5 * time.Second)
				continue
			}

			list := fetcher.FetchAll(cfg)
			storage.SaveProjectStatus(list)
			time.Sleep(5 * time.Second)
		}
	}()

	// 启动 HTTP
	web.StartHTTP()
}
