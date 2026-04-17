// cmd/fins/commands/workspace.go

package commands

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"finsd/internal/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type WorkspaceConfig struct {
	Name string `mapstructure:"name" yaml:"name"`
	Path string `mapstructure:"path" yaml:"path"`
}

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage local build workspaces",
}

var workspaceAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Add current directory as a workspace",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		absPath, _ := filepath.Abs(".")

		var workspaces []WorkspaceConfig
		if err := viper.UnmarshalKey("local_packages", &workspaces); err != nil {
			utils.LogError(os.Stdout, "Failed to parse workspaces: %v", err)
			return
		}

		for _, ws := range workspaces {
			if ws.Name == name {
				utils.LogError(os.Stdout, "Workspace with name '%s' already exists (Path: %s)", name, ws.Path)
				return
			}
			if ws.Path == absPath {
				utils.LogError(os.Stdout, "This directory is already added as workspace '%s'", ws.Name)
				return
			}
		}

		workspaces = append(workspaces, WorkspaceConfig{Name: name, Path: absPath})
		viper.Set("local_packages", workspaces)

		if err := viper.WriteConfig(); err != nil {
			utils.LogError(os.Stdout, "Failed to save config: %v", err)
			return
		}

		utils.LogSuccess(os.Stdout, "Workspace '%s' added at %s", name, absPath)

		url := fmt.Sprintf("%s/api/scan", DaemonURL)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			utils.LogWarning(os.Stdout, "Added workspace, but failed to notify daemon for rescan: %v", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				utils.LogSuccess(os.Stdout, "Daemon triggered automatic scan.")
			} else {
				utils.LogWarning(os.Stdout, "Daemon returned %d on scan request.", resp.StatusCode)
			}
		}
	},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered workspaces",
	Run: func(cmd *cobra.Command, args []string) {
		var workspaces []WorkspaceConfig
		if err := viper.UnmarshalKey("local_packages", &workspaces); err != nil {
			utils.LogError(os.Stdout, "Failed to parse workspaces: %v", err)
			return
		}

		if len(workspaces) == 0 {
			utils.LogWarning(os.Stdout, "No workspaces registered.")
			return
		}

		fmt.Println("Registered Workspaces:")
		fmt.Printf("%-20s %-50s\n", "Name", "Path")
		fmt.Println(strings.Repeat("-", 75))
		for _, ws := range workspaces {
			fmt.Printf("%-20s %-50s\n", ws.Name, ws.Path)
		}
	},
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a workspace from registration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		var workspaces []WorkspaceConfig
		if err := viper.UnmarshalKey("local_packages", &workspaces); err != nil {
			utils.LogError(os.Stdout, "Failed to parse workspaces: %v", err)
			return
		}

		var newWorkspaces []WorkspaceConfig
		found := false
		for _, ws := range workspaces {
			if ws.Name == name {
				found = true
				continue
			}
			newWorkspaces = append(newWorkspaces, ws)
		}

		if !found {
			utils.LogError(os.Stdout, "Workspace '%s' not found.", name)
			return
		}

		viper.Set("local_packages", newWorkspaces)
		if err := viper.WriteConfig(); err != nil {
			utils.LogError(os.Stdout, "Failed to save config: %v", err)
			return
		}

		utils.LogSuccess(os.Stdout, "Workspace '%s' removed.", name)
	},
}

var workspaceScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Manually trigger a scan of all registered workspaces",
	Run: func(cmd *cobra.Command, args []string) {
		url := fmt.Sprintf("%s/api/scan", DaemonURL)
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			utils.LogError(os.Stdout, "Failed to connect to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			utils.LogSuccess(os.Stdout, "Package scan triggered successfully.")
		} else {
			utils.LogError(os.Stdout, "Failed to trigger scan: Daemon returned %d", resp.StatusCode)
		}
	},
}

func init() {
	workspaceCmd.AddCommand(workspaceAddCmd, workspaceListCmd, workspaceRemoveCmd, workspaceScanCmd)
	RootCmd.AddCommand(workspaceCmd)
}
