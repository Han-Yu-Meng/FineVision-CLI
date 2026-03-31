// cmd/fins/commands/status.go

package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"finsd/internal/types"
	"finsd/internal/utils"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "List packages and status",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := http.Get(DaemonURL + "/api/packages")
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd. Is it running?")
			return
		}
		defer resp.Body.Close()

		var pkgs []types.PackageInfo
		if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
			utils.LogError(os.Stdout, "Error decoding response: %v", err)
			return
		}

		fmt.Printf("%-30s %-10s %-15s %s\n", "PACKAGE", "VERSION", "SOURCE", "STATUS")
		fmt.Println(strings.Repeat("-", 80))
		for _, p := range pkgs {
			c := color.New(color.FgWhite)
			if p.Status == types.StatusCurrent {
				c = color.New(color.FgGreen)
			} else if p.Status == types.StatusStale {
				c = color.New(color.FgYellow)
			} else if p.Status == types.StatusCompiling {
				c = color.New(color.FgCyan)
			} else if p.Status == types.StatusFailed {
				c = color.New(color.FgRed)
			}

			// p.Name is the unique ID from scanner (source/name), convert to short name for display
			parts := strings.Split(p.Name, "/")
			displayName := parts[len(parts)-1]
			c.Printf("%-30s %-10s %-15s %s\n", displayName, p.Version, p.Source, p.Status)
		}
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
