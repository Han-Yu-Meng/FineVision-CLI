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

// pullGitUpdates checks for updates in a git repository directory.
// Returns (updated bool, err error).
func pullGitUpdates(dir string, name string) (bool, error) {
	headBefore, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	headBefore = bytes.TrimSpace(headBefore)

	utils.LogSection(os.Stdout, "Pulling latest %s updates...", name)

	gitPull := exec.Command("git", "-C", dir, "pull", "--ff-only")
	gitPull.Stdout = os.Stdout
	gitPull.Stderr = os.Stderr

	if err := gitPull.Run(); err != nil {
		return false, err
	}

	headAfter, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	headAfter = bytes.TrimSpace(headAfter)

	return !bytes.Equal(headBefore, headAfter), nil
}

// updateLaunch pulls and reinstalls FineVision-Launch if updated.
func updateLaunch(launchDir string, forceRebuild bool) {
	if _, err := os.Stat(launchDir); os.IsNotExist(err) {
		utils.LogInfo(os.Stdout, "Launch directory not found at %s, skipping.", launchDir)
		return
	}

	updated, err := pullGitUpdates(launchDir, "Launch")
	if err != nil {
		utils.LogWarning(os.Stdout, "Launch git pull failed: %v", err)
		return
	}

	if !updated && !forceRebuild {
		utils.LogSuccess(os.Stdout, "Launch is already up to date.")
		return
	}

	if forceRebuild {
		utils.LogInfo(os.Stdout, "--rebuild: forcing Launch reinstall.")
	} else {
		utils.LogSuccess(os.Stdout, "Launch updated, reinstalling...")
	}

	utils.LogSection(os.Stdout, "Installing FineVision-Launch...")
	pipArgs := []string{"install", "--user"}

	// Check if --break-system-packages is needed (Ubuntu 23.04+)
	ubunutVersion, _ := exec.Command("lsb_release", "-rs").Output()
	ubunutVersion = bytes.TrimSpace(ubunutVersion)
	if len(ubunutVersion) > 0 {
		// Simple version comparison: "23.04" >= "23.04"
		v := string(ubunutVersion)
		if v >= "23.04" {
			pipArgs = append(pipArgs, "--break-system-packages")
		}
	}

	pipArgs = append(pipArgs, ".")
	pipCmd := exec.Command("pip3", pipArgs...)
	pipCmd.Dir = launchDir
	pipCmd.Stdout = os.Stdout
	pipCmd.Stderr = os.Stderr

	if err := pipCmd.Run(); err != nil {
		utils.LogWarning(os.Stdout, "Launch pip install failed: %v", err)
		return
	}

	utils.LogSuccess(os.Stdout, "FineVision-Launch installed successfully.")
}

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
		launchDir := filepath.Join(utils.GetFinsHome(), "launch")

		if _, err := os.Stat(sdkDir); os.IsNotExist(err) {
			utils.LogError(os.Stdout, "SDK directory not found at %s. Please run install.sh first.", sdkDir)
			return
		}

		// Step 1: Pull SDK updates
		sdkUpdated, err := pullGitUpdates(sdkDir, "SDK")
		if err != nil {
			utils.LogError(os.Stdout, "SDK git pull failed: %v", err)
			return
		}

		// Step 2: Pull and reinstall Launch
		updateLaunch(launchDir, updateRebuild)

		if !updateRebuild && !sdkUpdated {
			utils.LogSuccess(os.Stdout, "SDK is already up to date. Nothing to do.")
			return
		}

		if updateRebuild {
			utils.LogInfo(os.Stdout, "--rebuild: forcing clean rebuild.")
		} else {
			utils.LogSuccess(os.Stdout, "SDK updated, rebuilding...")
		}

		// Step 3: Clean all build artifacts
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

		// Step 4: Rebuild SDK static library
		ctx := context.Background()
		utils.LogSection(os.Stdout, "Rebuilding SDK static library...")
		if err := core.CompileSDKStatic(ctx, os.Stdout); err != nil {
			utils.LogError(os.Stdout, "SDK static library build failed: %v", err)
			return
		}

		// Step 5: Rebuild agent
		utils.LogSection(os.Stdout, "Rebuilding agent...")
		if err := core.CompileAgent(ctx, os.Stdout); err != nil {
			utils.LogError(os.Stdout, "Agent build failed: %v", err)
			return
		}

		// Step 6: Rebuild inspect
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
