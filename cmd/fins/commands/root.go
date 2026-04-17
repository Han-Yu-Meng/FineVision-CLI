package commands

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const DaemonURL = "http://localhost:8899"
const MaxConcurrentBuilds = 4

var RootCmd = &cobra.Command{Use: "fins"}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()
	viper.AddConfigPath(filepath.Join(home, ".fins"))
	viper.AddConfigPath(".")

	viper.SetConfigName("config")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SetConfigName("config_default")
			viper.ReadInConfig()
		}
	}
}
