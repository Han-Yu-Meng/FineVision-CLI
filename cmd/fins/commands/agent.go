// fins/cmd/fins/commands/agent.go

package commands

import (
	"fmt"
	"net/http"
	"os"

	"finsd/cmd/fins/client"
	"finsd/internal/utils"

	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage the fins agent",
}

var agentBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the fins agent binary",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		url := fmt.Sprintf("%s/api/agent/build", DaemonURL)

		utils.LogSection(os.Stdout, "Requesting agent build")
		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		client.StreamResponse(resp.Body)
	},
}

func init() {
	agentCmd.AddCommand(agentBuildCmd)
	RootCmd.AddCommand(agentCmd)
}
