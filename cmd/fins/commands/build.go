package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fins-cli/cmd/fins/client"
	"fins-cli/internal/utils"

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

		var targetPkg string
		if len(args) == 0 || args[0] == "." {
			absPath, _ := filepath.Abs(".")
			var workspaces []WorkspaceConfig
			if err := viper.UnmarshalKey("local_packages", &workspaces); err == nil {
				for _, ws := range workspaces {
					if ws.Path == absPath {
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
				if strings.Contains(status, "Building") {
					states[idx].startTime = time.Now()
				} else if strings.Contains(status, "Success") || strings.Contains(status, "Failed") {
					states[idx].endTime = time.Now()
				}
				mu.Unlock()
			}

			done := make(chan struct{})

			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			ctx, cancel := context.WithCancel(context.Background())

			go func() {
				select {
				case <-done:
					return
				case <-sigChan:
					cancel()
					return
				}
			}()

			var wg sync.WaitGroup
			sem := make(chan struct{}, MaxConcurrentBuilds)

			var errMu sync.Mutex
			var errorLogs []string

			for i, p := range pkgs {
				wg.Add(1)
				go func(idx int, pkgName string) {
					defer wg.Done()

					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						updateStatus(idx, color.YellowString("Cancelled ⚠"))
						fmt.Printf("[%s] - %s\n", pkgName, color.YellowString("Cancelled"))
						return
					}

					updateStatus(idx, color.CyanString("Building 🚀"))
					fmt.Printf("[%s] - %s\n", pkgName, color.CyanString("Building..."))

					url := fmt.Sprintf("%s/api/build/%s", DaemonURL, pkgName)

					req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
					resp, err := http.DefaultClient.Do(req)

					defer func() { <-sem }()

					if err != nil {
						if ctx.Err() != nil {
							updateStatus(idx, color.YellowString("Cancelled ⚠"))
						} else {
							updateStatus(idx, color.RedString("Request Failed ✘"))
							fmt.Printf("[%s] - %s\n", pkgName, color.RedString("Request Failed"))
							errMu.Lock()
							errorLogs = append(errorLogs, fmt.Sprintf(">>> Package: %s (Request Failed)\n%v\n", pkgName, err))
							errMu.Unlock()
						}
						return
					}
					defer resp.Body.Close()

					output, _ := io.ReadAll(resp.Body)
					outStr := string(output)

					mu.Lock()
					s := states[idx]
					elapsed := ""
					if !s.startTime.IsZero() {
						d := time.Since(s.startTime)
						elapsed = fmt.Sprintf("%.1fs", d.Seconds())
					}
					mu.Unlock()

					if strings.Contains(outStr, "[ERROR]") {
						updateStatus(idx, color.RedString("Failed ✘"))
						fmt.Printf("[%s] - %s (%s)\n", pkgName, color.RedString("Failed"), elapsed)
						errMu.Lock()
						errorLogs = append(errorLogs, fmt.Sprintf(">>> Package: %s (Build Failed)\n%s\n", pkgName, outStr))
						errMu.Unlock()
					} else {
						updateStatus(idx, color.GreenString("Success ✔"))
						fmt.Printf("[%s] - %s (%s)\n", pkgName, color.GreenString("Success"), elapsed)
					}
				}(i, p.Name)
			}

			wg.Wait()
			close(done)

			if len(errorLogs) > 0 {
				utils.LogError(os.Stdout, "Errors Encountered:")
				for _, log := range errorLogs {
					fmt.Println(log)
					fmt.Println(strings.Repeat("-", 40))
				}
				utils.LogError(os.Stdout, "Tasks completed with errors")
			} else {
				if ctx.Err() != nil {
					utils.LogWarning(os.Stdout, "Build interrupted by user")
				} else {
					utils.LogSuccess(os.Stdout, "All tasks completed successfully")
				}
			}
			return
		}

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

		client.StreamResponse(resp.Body)
	},
}

func init() {
	buildCmd.Flags().BoolVar(&buildAll, "all", false, "Build all packages in parallel")
	buildCmd.Flags().BoolVar(&buildClear, "clear", false, "Clear all build caches before building")
	buildCmd.Flags().StringVar(&targetSource, "source", "", "Specify package source to resolve ambiguity")
	RootCmd.AddCommand(buildCmd)
}
