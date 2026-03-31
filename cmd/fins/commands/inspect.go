// cmd/fins/commands/inspect.go

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"finsd/cmd/fins/client"
	"finsd/internal/utils"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [package]",
	Short: "Inspect a compiled package binary",
	Long:  `Analyze the shared object (.so) file of a package to reveal its architecture, dependencies, and FINS nodes.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkgName := args[0]

		// 1. 兼容性保护：防止拦截子命令
		if pkgName == "build" {
			inspectBuildCmd.Run(cmd, args)
			return
		}

		// 2. 使用 resolver 解析包名
		finalPkg, err := client.ResolvePackageIdentity(DaemonURL, pkgName, targetSource)
		if err != nil {
			utils.LogError(os.Stdout, "%v", err)

			return
		}

		// 3. 执行 Inspect 请求
		url := fmt.Sprintf("%s/api/inspect/analyze/%s", DaemonURL, finalPkg)

		resp, err := http.Get(url)
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			// 如果后端返回了 JSON 错误信息，尝试美化，否则直接打印
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
	},
}

var inspectBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the inspect tool binary",
	Run: func(cmd *cobra.Command, args []string) {
		url := fmt.Sprintf("%s/api/inspect/build", DaemonURL)

		utils.LogSection(os.Stdout, "Requesting inspect tool build")
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
	inspectCmd.AddCommand(inspectBuildCmd)
	// 复用 compile.go 中定义的 targetSource 变量
	inspectCmd.Flags().StringVar(&targetSource, "source", "", "Specify package source to resolve ambiguity")
	RootCmd.AddCommand(inspectCmd)
}

func printColoredJSON(data []byte) {
	fmt.Println(string(data))
}
