package utils

import (
	"os"
	"path/filepath"
	"strings"
)

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

func GetLogDir() string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".fins", "logs")
	os.MkdirAll(logDir, 0755)
	return logDir
}

func GetROSDistro() string {
	distros := []string{"jazzy", "humble", "iron", "noetic", "galactic", "foxy"}

	if envDistro := os.Getenv("ROS_DISTRO"); envDistro != "" {
		return envDistro
	}

	for _, d := range distros {
		if _, err := os.Stat(filepath.Join("/opt/ros", d)); err == nil {
			return d
		}
	}

	return ""
}
