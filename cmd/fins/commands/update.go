package commands

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"fins-cli/internal/core"
	"fins-cli/internal/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	updateRebuild bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update SDK and rebuild all components",
	Long: `Pull the latest FineVision SDK from git and rebuild.

This command runs 'git pull' in the SDK directory (~/.fins/sdk).
If new changes are pulled, it will:
  1. Clean all local build artifacts and package build directories
  2. Rebuild the SDK static library
  3. Rebuild the agent and inspect executables

If the SDK is already up to date, no rebuild is performed.

Use --rebuild to force clean and rebuild regardless of whether
the SDK was updated.`,
	Run: func(cmd *cobra.Command, args []string) {
		sdkDir := filepath.Join(utils.GetFinsHome(), "sdk")

		if _, err := os.Stat(sdkDir); os.IsNotExist(err) {
			utils.LogError(os.Stdout, "SDK directory not found at %s. Please run install.sh first.", sdkDir)
			return
		}

		// Step 1: Record HEAD before pull
		headBefore, _ := exec.Command("git", "-C", sdkDir, "rev-parse", "HEAD").Output()
		headBefore = bytes.TrimSpace(headBefore)

		// Step 2: git pull
		utils.LogSection(os.Stdout, "Pulling latest SDK updates...")

		gitPull := exec.Command("git", "-C", sdkDir, "pull", "--ff-only")
		gitPull.Stdout = os.Stdout
		gitPull.Stderr = os.Stderr

		if err := gitPull.Run(); err != nil {
			utils.LogError(os.Stdout, "Git pull failed: %v", err)
			return
		}

		// Step 3: Compare HEAD to detect actual changes
		headAfter, _ := exec.Command("git", "-C", sdkDir, "rev-parse", "HEAD").Output()
		headAfter = bytes.TrimSpace(headAfter)

		sdkUpdated := !bytes.Equal(headBefore, headAfter)

		if !updateRebuild && !sdkUpdated {
			utils.LogSuccess(os.Stdout, "SDK is already up to date. Nothing to do.")
			return
		}

		if updateRebuild {
			utils.LogInfo(os.Stdout, "--rebuild: forcing clean rebuild.")
		} else {
			utils.LogSuccess(os.Stdout, "SDK updated, rebuilding...")
		}

		// Step 4: Clean all build artifacts
		utils.LogSection(os.Stdout, "Cleaning all build artifacts...")

		// Clean package build directories and core build dir
		if err := core.CleanAllBuilds(); err != nil {
			utils.LogWarning(os.Stdout, "Warning during clean: %v", err)
		}

		// Clean SDK static build directory
		sdkBuildDir := filepath.Join(utils.GetFinsHome(), "build", "sdk_static")
		if _, err := os.Stat(sdkBuildDir); err == nil {
			utils.LogInfo(os.Stdout, "Cleaning %s", sdkBuildDir)
			os.RemoveAll(sdkBuildDir)
		}

		// Clean install directory to remove stale artifacts
		installDir := utils.ExpandPath(viper.GetString("build.defaults.build_output"))
		if _, err := os.Stat(installDir); err == nil {
			utils.LogInfo(os.Stdout, "Cleaning install directory: %s", installDir)
			os.RemoveAll(installDir)
		}

		utils.LogSuccess(os.Stdout, "Build artifacts cleaned.")

		// Step 5: Rebuild SDK static library
		ctx := context.Background()
		utils.LogSection(os.Stdout, "Rebuilding SDK static library...")
		if err := core.CompileSDKStatic(ctx, os.Stdout); err != nil {
			utils.LogError(os.Stdout, "SDK static library build failed: %v", err)
			return
		}

		// Step 6: Rebuild agent
		utils.LogSection(os.Stdout, "Rebuilding agent...")
		if err := core.CompileAgent(ctx, os.Stdout); err != nil {
			utils.LogError(os.Stdout, "Agent build failed: %v", err)
			return
		}

		// Step 7: Rebuild inspect
		utils.LogSection(os.Stdout, "Rebuilding inspect...")
		if err := core.CompileInspect(ctx, os.Stdout); err != nil {
			utils.LogError(os.Stdout, "Inspect build failed: %v", err)
			return
		}

		utils.LogSuccess(os.Stdout, "Update complete! SDK and core executables rebuilt successfully.")
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateRebuild, "rebuild", false, "Force clean and rebuild even if SDK is already up to date")
	RootCmd.AddCommand(updateCmd)
}
