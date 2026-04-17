package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"fins-cli/internal/utils"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install [GIT_REPO]",
	Short: "Install a plugin from a GitHub repository release",
	Long:  "Request daemon to download the latest release from a GitHub repository, extract the appropriate .so plugin, and install it to ~/.fins/install/.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repo := args[0]

		url := fmt.Sprintf("%s/api/install", DaemonURL)
		payload, _ := json.Marshal(map[string]string{
			"repo": repo,
		})

		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payload))
		if err != nil {
			utils.LogError(os.Stdout, "Failed to connect to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			utils.LogError(os.Stdout, "Failed to install: %s", string(body))
			return
		}

		_, err = io.Copy(os.Stdout, resp.Body)
		if err != nil {
			utils.LogError(os.Stdout, "Error reading response: %v", err)
		}
	},
}

func init() {
	RootCmd.AddCommand(installCmd)
}
