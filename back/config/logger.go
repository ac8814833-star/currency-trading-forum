package config

import (
	"github.com/sirupsen/logrus"
	"os"
)

func InitLogger() {
	// 设置日志格式为 JSON 格式
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// 设置日志输出到标准输出（可以配置为文件）
	logrus.SetOutput(os.Stdout)

	// 设置日志级别
	logrus.SetLevel(logrus.InfoLevel)
}
