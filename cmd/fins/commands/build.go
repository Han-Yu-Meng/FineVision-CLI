// cmd/fins/commands/compile.go

package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"finsd/cmd/fins/client"
	"finsd/internal/utils"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	buildAll     bool
	buildClear   bool
	targetSource string
)

var buildCmd = &cobra.Command{
	Use:   "build [package]",
	Short: "Request daemon to build a package",
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		pkgs, err := client.FetchPackageList(DaemonURL)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		var suggestions []string
		for _, p := range pkgs {
			suggestions = append(suggestions, p.Name)
		}
		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
	Args: func(cmd *cobra.Command, args []string) error {
		if buildClear {
			return nil
		}
		if buildAll {
			if len(args) > 0 {
				return fmt.Errorf("cannot use arguments with --all")
			}
			return nil
		}
		if len(args) == 0 {
			return nil
		}
		if len(args) != 1 {
			return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if buildClear {
			url := fmt.Sprintf("%s/api/clean", DaemonURL)
			resp, err := http.Post(url, "application/json", nil)
			if err != nil {
				utils.LogError(os.Stdout, "Failed to connect to finsd: %v", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				utils.LogSuccess(os.Stdout, "All build caches cleared successfully")
			} else {
				body, _ := io.ReadAll(resp.Body)
				utils.LogError(os.Stdout, "Failed to clean cache: %s", string(body))
			}

			if !buildAll && len(args) == 0 {
				return
			}
		}

		// Handle "fins build ." or "fins build" in a workspace directory
		var targetPkg string
		if len(args) == 0 || args[0] == "." {
			absPath, _ := filepath.Abs(".")
			var workspaces []WorkspaceConfig
			if err := viper.UnmarshalKey("local_packages", &workspaces); err == nil {
				for _, ws := range workspaces {
					if ws.Path == absPath {
						// Found current directory in registered workspaces, trigger build all
						buildAll = true
						break
					}
				}
			}

			if !buildAll {
				if len(args) == 0 {
					utils.LogError(os.Stdout, "Package name required or run in a registered workspace.")
					return
				}
				// If "." but not a workspace, it might be a relative path to a single package?
				// For now, let's assume the user meant a single package if not buildAll.
				targetPkg = args[0]
			}
		} else {
			targetPkg = args[0]
		}

		if buildAll {
			pkgs, err := client.FetchPackageList(DaemonURL)
			if err != nil {
				utils.LogError(os.Stdout, "Failed to connect to daemon: %v", err)
				return
			}

			n := len(pkgs)
			type taskState struct {
				status    string
				startTime time.Time
				endTime   time.Time
			}
			states := make([]taskState, n)
			for i := range states {
				states[i].status = "Pending ⏳"
			}

			var mu sync.Mutex
			updateStatus := func(idx int, status string) {
				mu.Lock()
				states[idx].status = status
				if strings.Contains(status, "Compiling") {
					states[idx].startTime = time.Now()
				} else if strings.Contains(status, "Success") || strings.Contains(status, "Failed") {
					states[idx].endTime = time.Now()
				}
				mu.Unlock()
			}

			done := make(chan bool)
			ticker := time.NewTicker(100 * time.Millisecond)

			fmt.Printf("%-30s %-10s %-15s %-10s %s\n", "PACKAGE", "VERSION", "SOURCE", "ELAPSED", "STATUS")
			fmt.Println(strings.Repeat("-", 100))

			for i := 0; i < n; i++ {
				fmt.Println()
			}

			printList := func() {
				mu.Lock()
				fmt.Printf("\033[%dA", n)
				for i, p := range pkgs {
					s := states[i]
					elapsed := ""
					if !s.startTime.IsZero() {
						d := time.Since(s.startTime)
						if !s.endTime.IsZero() {
							d = s.endTime.Sub(s.startTime)
						}
						elapsed = fmt.Sprintf("%.1fs", d.Seconds())
					}

					displayName := p.Name
					fmt.Printf("\033[2K\r%-30s %-10s %-15s %-10s %s\n", displayName, p.Version, p.Source, elapsed, s.status)
				}
				mu.Unlock()
			}

			go func() {
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						printList()
					}
				}
			}()

			// 任务队列
			var wg sync.WaitGroup
			sem := make(chan struct{}, MaxConcurrentBuilds) // 信号量限制并发数

			// 错误日志收集
			var errMu sync.Mutex
			var errorLogs []string

			for i, p := range pkgs {
				wg.Add(1)
				go func(idx int, pkgName string) {
					defer wg.Done()

					sem <- struct{}{} // 获取令牌
					updateStatus(idx, color.CyanString("Building 🚀"))

					url := fmt.Sprintf("%s/api/build/%s", DaemonURL, pkgName)
					resp, err := http.Post(url, "application/json", nil)

					defer func() { <-sem }() // 释放令牌

					if err != nil {
						updateStatus(idx, color.RedString("Request Failed ✘"))
						errMu.Lock()
						errorLogs = append(errorLogs, fmt.Sprintf(">>> Package: %s (Request Failed)\n%v\n", pkgName, err))
						errMu.Unlock()
						return
					}
					defer resp.Body.Close()

					output, _ := io.ReadAll(resp.Body)
					outStr := string(output)

					if strings.Contains(outStr, "[ERROR]") {
						updateStatus(idx, color.RedString("Failed ✘"))
						errMu.Lock()
						errorLogs = append(errorLogs, fmt.Sprintf(">>> Package: %s (Build Failed)\n%s\n", pkgName, outStr))
						errMu.Unlock()
					} else {
						updateStatus(idx, color.GreenString("Success ✔"))
					}
				}(i, p.Name)
			}

			wg.Wait()
			ticker.Stop()
			done <- true

			// 最后刷新一次确保状态一致
			time.Sleep(200 * time.Millisecond)
			printList()
			fmt.Println(strings.Repeat("-", 90))

			if len(errorLogs) > 0 {
				utils.LogError(os.Stdout, "Errors Encountered:")
				for _, log := range errorLogs {
					fmt.Println(log)
					fmt.Println(strings.Repeat("-", 40))
				}
				utils.LogError(os.Stdout, "Tasks completed with errors")
			} else {
				utils.LogSuccess(os.Stdout, "All tasks completed successfully")
			}
			return
		}

		// 使用 resolver 解析包名
		finalPkg, err := client.ResolvePackageIdentity(DaemonURL, targetPkg, targetSource)
		if err != nil {
			utils.LogError(os.Stdout, "%v", err)
			return
		}

		url := fmt.Sprintf("%s/api/build/%s", DaemonURL, finalPkg)

		resp, err := http.Post(url, "application/json", nil)
		if err != nil {
			utils.LogError(os.Stdout, "Error connecting to finsd: %v", err)
			return
		}
		defer resp.Body.Close()

		// 使用流式输出，实时显示编译进度
		client.StreamResponse(resp.Body)
	},
}

func init() {
	buildCmd.Flags().BoolVar(&buildAll, "all", false, "Build all packages in parallel")
	buildCmd.Flags().BoolVar(&buildClear, "clear", false, "Clear all build caches before building")
	buildCmd.Flags().StringVar(&targetSource, "source", "", "Specify package source to resolve ambiguity")
	RootCmd.AddCommand(buildCmd)
}
