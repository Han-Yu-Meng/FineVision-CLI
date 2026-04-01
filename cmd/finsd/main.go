package main

import (
	"os"

	"finsd/internal/monitor"
	"finsd/internal/server"
	"finsd/internal/server/handlers"
	"finsd/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func main() {
	// 初始化配置
	viper.AddConfigPath("$HOME/.fins")
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	if err := viper.ReadInConfig(); err != nil {
		utils.LogWarning(os.Stdout, "Failed to read config: %v", err)
	} else {
		utils.LogSuccess(os.Stdout, "Using config file: %s", viper.ConfigFileUsed())
	}

	// 初始化包监视器
	var err error
	handlers.PackageWatcher, err = monitor.NewWatcher()
	if err != nil {
		panic(err)
	}
	handlers.PackageWatcher.Start()

	// 创建 Gin 路由器
	r := gin.Default()

	// 设置路由
	server.SetupRoutes(r)

	// 启动服务器
	r.Run(":8899")
}
