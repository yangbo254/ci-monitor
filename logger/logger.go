package logger

import (
	"log"
	"os"
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

	Info = log.New(file, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(file, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	Debug = log.New(file, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

	// 同时输出到控制台
	Info.SetOutput(os.Stdout)
	Error.SetOutput(os.Stdout)
	Debug.SetOutput(os.Stdout)
}
