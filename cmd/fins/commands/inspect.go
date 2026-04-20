package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fins-cli/cmd/fins/client"
	"fins-cli/internal/core"
	"fins-cli/internal/utils"

	"github.com/spf13/cobra"
)

var (
	inspectFile string
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [package]",
	Short: "Inspect a compiled package binary",
	Long:  `Analyze the shared object (.so) file of a package to reveal its architecture, dependencies, and FINS nodes.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if inspectFile != "" {
			var targetPath string

			if filepath.IsAbs(inspectFile) {
				targetPath = inspectFile
			} else {
				matches, _ := filepath.Glob(inspectFile)
				if len(matches) == 1 {
					targetPath = matches[0]
				} else if len(matches) > 1 {
					utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match '%s' in current directory", len(matches), inspectFile)
					return
				} else {
					home, _ := os.UserHomeDir()
					installDir := filepath.Join(home, ".fins/install")
					pattern := filepath.Join(installDir, inspectFile)
					installMatches, _ := filepath.Glob(pattern)

					if len(installMatches) == 0 {
						utils.LogError(os.Stdout, "No file found matching '%s' in current directory or %s", inspectFile, installDir)
						return
					}
					if len(installMatches) > 1 {
						var foundNames []string
						for _, m := range installMatches {
							foundNames = append(foundNames, filepath.Base(m))
						}
						utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match in %s: %v", len(installMatches), installDir, foundNames)
						return
					}
					targetPath = installMatches[0]
				}
			}

			if strings.ContainsAny(targetPath, "*?[]") {
				matches, _ := filepath.Glob(targetPath)
				if len(matches) == 0 {
					utils.LogError(os.Stdout, "No file found matching pattern: %s", targetPath)
					return
				}
				if len(matches) > 1 {
					utils.LogError(os.Stdout, "Ambiguous file pattern: %d files match: %v", len(matches), matches)
					return
				}
				targetPath = matches[0]
			}

			absPath, err := filepath.Abs(targetPath)
			if err != nil {
				utils.LogError(os.Stdout, "Failed to resolve absolute path: %v", err)
				return
			}

			u, _ := url.Parse(fmt.Sprintf("%s/api/inspect/file", DaemonURL))
			q := u.Query()
			q.Set("path", absPath)
			u.RawQuery = q.Encode()

			resp, err := http.Get(u.String())
			if err != nil {
				utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
				return
			}
			defer resp.Body.Close()

			handleInspectResponse(resp)
			return
		}

		if len(args) == 0 {
			cmd.Help()
			return
		}

		pkgName := args[0]

		if pkgName == "build" {
			inspectBuildCmd.Run(cmd, args)
			return
		}

		finalPkg, err := client.ResolvePackageIdentity(DaemonURL, pkgName, targetSource)
		if err != nil {
			utils.LogError(os.Stdout, "%v", err)

			return
		}

		apiURL := fmt.Sprintf("%s/api/inspect/analyze/%s", DaemonURL, finalPkg)

		resp, err := http.Get(apiURL)
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		handleInspectResponse(resp)
	},
}

func handleInspectResponse(resp *http.Response) {
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		var errObj map[string]interface{}
		if json.Unmarshal(body, &errObj) == nil {
			if msg, ok := errObj["error"].(string); ok {
				utils.LogError(os.Stdout, "Failed to inspect package: %s", msg)
				return
			}
		}
		utils.LogError(os.Stdout, "Failed to inspect package:")
		fmt.Println(string(body))
		return
	}

	rawJSON, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.LogError(os.Stdout, "Failed to read response: %v", err)
		return
	}

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawJSON, "", "    "); err != nil {
		fmt.Println(string(rawJSON))
		return
	}

	printColoredJSON(prettyJSON.Bytes())
}

var inspectBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the inspect tool binary",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		utils.LogSection(os.Stdout, "Building inspect tool binary")

		err := core.CompileInspect(context.Background(), os.Stdout)
		if err != nil {
			utils.LogError(os.Stdout, "Build failed: %v", err)
			return
		}
		utils.LogSuccess(os.Stdout, "Inspect tool built successfully.")
	},
}

func init() {
	inspectCmd.AddCommand(inspectBuildCmd)
	inspectCmd.Flags().StringVar(&targetSource, "source", "", "Specify package source to resolve ambiguity")
	inspectCmd.Flags().StringVar(&inspectFile, "file", "", "Analyze a specific .so file directly")
	RootCmd.AddCommand(inspectCmd)
}

func printColoredJSON(data []byte) {
	fmt.Println(string(data))
}
