package main

import (
	"os"

	"fins-cli/internal/monitor"
	"fins-cli/internal/server"
	"fins-cli/internal/server/handlers"
	"fins-cli/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func main() {
	viper.AddConfigPath(utils.GetFinsHome())
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
