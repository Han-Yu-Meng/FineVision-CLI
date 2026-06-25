package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"

	"fins-cli/internal/utils"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration and build presets",
}

var configPresetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Manage build presets",
}

var configPresetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available build presets",
	Run: func(cmd *cobra.Command, args []string) {
		current := viper.GetString("build.default_preset")
		presets := viper.GetStringMap("build.presets")

		if len(presets) == 0 {
			utils.LogWarning(os.Stdout, "No presets found in configuration.")
			return
		}

		var names []string
		for k := range presets {
			names = append(names, k)
		}
		sort.Strings(names)

		fmt.Println("Available Presets:")
		fmt.Println("------------------")
		for _, name := range names {
			data := viper.GetStringMap(fmt.Sprintf("build.presets.%s", name))
			desc, _ := data["description"].(string)

			prefix := "  "
			suffix := ""
			if name == current {
				prefix = "* "
				suffix = color.GreenString(" (current)")
			}

			fmt.Printf("%s%-12s %s%s\n", prefix, name, desc, suffix)
		}
	},
}

var configPresetSetCmd = &cobra.Command{
	Use:   "set [preset_name]",
	Short: "Set the default build preset",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		// 🌟 1. 向 Daemon 请求可用预设列表进行校验
		respList, err := http.Get(fmt.Sprintf("%s/api/presets", DaemonURL))
		if err != nil {
			utils.LogError(os.Stdout, "Failed to connect to daemon: %v", err)
			return
		}
		defer respList.Body.Close()

		var presetData struct {
			Presets []string `json:"presets"`
		}
		if err := json.NewDecoder(respList.Body).Decode(&presetData); err != nil {
			utils.LogError(os.Stdout, "Failed to parse presets from daemon: %v", err)
			return
		}

		found := false
		for _, p := range presetData.Presets {
			if p == name {
				found = true
				break
			}
		}
		if !found {
			utils.LogError(os.Stdout, "Preset '%s' does not exist. Available: %v", name, presetData.Presets)
			return
		}

		// 🌟 2. 发送 POST 请求直接命令 Daemon 修改预设，确保内存与磁盘配置文件同步更新
		payload, _ := json.Marshal(map[string]string{"name": name})
		url := fmt.Sprintf("%s/api/preset", DaemonURL)
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			utils.LogError(os.Stdout, "Failed to update preset on daemon: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			utils.LogSuccess(os.Stdout, "Default preset successfully updated to '%s' on daemon.", name)
		} else {
			utils.LogError(os.Stdout, "Daemon failed to update preset (status %d)", resp.StatusCode)
		}
	},
}

func init() {
	configPresetCmd.AddCommand(configPresetListCmd, configPresetSetCmd)
	configCmd.AddCommand(configPresetCmd)
	RootCmd.AddCommand(configCmd)
}
