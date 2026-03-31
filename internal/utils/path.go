package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath 将路径中的 ~ 替换为当前用户的主目录
func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}

		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}

	return path
}

// GetLogDir 获取日志存储目录并确保其存在
func GetLogDir() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".fins", "logs")
	os.MkdirAll(logDir, 0755)
	return logDir
}
