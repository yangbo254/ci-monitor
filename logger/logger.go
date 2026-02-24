package logger

import (
	"io"
	"log"
	"os"
	"strings"
)

var (
	Info  *log.Logger
	Error *log.Logger
	Debug *log.Logger
)

func Init(logFile string) {
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("无法打开日志文件: %v", err)
	}

	level := strings.ToLower(strings.TrimSpace(os.Getenv("CI_MONITOR_LOG_LEVEL")))
	if level == "" {
		level = "info"
	}

	consoleAndFile := io.MultiWriter(file, os.Stdout)

	Info = log.New(consoleAndFile, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(consoleAndFile, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	Debug = log.New(consoleAndFile, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

	switch level {
	case "debug":
		// debug/info/error 全开
	case "error":
		Info.SetOutput(io.Discard)
		Debug.SetOutput(io.Discard)
	default:
		// 默认 info：关闭 debug，保留 info/error
		Debug.SetOutput(io.Discard)
	}
}
