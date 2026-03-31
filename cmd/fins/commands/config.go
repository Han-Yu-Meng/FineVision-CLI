// cmd/fins/commands/config.go

package commands

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"finsd/internal/utils"

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
		presets := viper.GetStringMap("build.presets")

		if _, ok := presets[name]; !ok {
			utils.LogError(os.Stdout, "Preset '%s' does not exist.", name)
			return
		}

		// Update in memory so subsequent commands in this session see it (if any)
		viper.Set("build.default_preset", name)

		// Manual update to file to preserve comments
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			utils.LogError(os.Stdout, "No configuration file found.")
			return
		}

		content, err := os.ReadFile(configFile)
		if err != nil {
			utils.LogError(os.Stdout, "Failed to read config file: %v", err)
			return
		}

		re := regexp.MustCompile(`(?m)^(  default_preset:\s*)(["']?)([^"'\n]+)(["']?)`)

		newContent := re.ReplaceAllString(string(content), fmt.Sprintf("${1}\"%s\"", name))

		if string(newContent) == string(content) {
			utils.LogWarning(os.Stdout, "Could not update config via regex, using Viper write instead.")
			if err := viper.WriteConfig(); err != nil {
				utils.LogError(os.Stdout, "Failed to save config: %v", err)
			}
		} else {
			if err := os.WriteFile(configFile, []byte(newContent), 0644); err != nil {
				utils.LogError(os.Stdout, "Failed to write config file: %v", err)
				return
			}
		}

		utils.LogSuccess(os.Stdout, "Default preset updated to '%s'", name)
	},
}

func init() {
	configPresetCmd.AddCommand(configPresetListCmd, configPresetSetCmd)
	configCmd.AddCommand(configPresetCmd)
	RootCmd.AddCommand(configCmd)
}
