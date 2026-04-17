package main

import (
	"os"
	"path/filepath"

	"finsd/internal/monitor"
	"finsd/internal/server"
	"finsd/internal/server/handlers"
	"finsd/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func main() {
	home, _ := os.UserHomeDir()
	viper.AddConfigPath(filepath.Join(home, ".fins"))
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	if err := viper.ReadInConfig(); err != nil {
		utils.LogWarning(os.Stdout, "Failed to read config: %v", err)
	} else {
		utils.LogSuccess(os.Stdout, "Using config file: %s", viper.ConfigFileUsed())
	}

	var err error
	handlers.PackageWatcher, err = monitor.NewWatcher()
	if err != nil {
		panic(err)
	}
	handlers.PackageWatcher.Start()

	r := gin.Default()
	server.SetupRoutes(r)
	r.Run(":8899")
}
