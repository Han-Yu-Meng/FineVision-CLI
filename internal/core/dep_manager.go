package core

import (
	"context"
	"fins-cli/internal/types"
	"fins-cli/internal/utils"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DepDirName = "dependencies"
)

func GetDepRoot() string {
	return filepath.Join(utils.GetFinsHome(), DepDirName)
}

func GetLogDir() string {
	return utils.GetLogDir()
}

func handlePPA(ctx context.Context, ppa string, writer io.Writer) error {
	if ppa == "" {
		return nil
	}

	utils.LogSection(writer, "Checking PPA: %s", ppa)

	// Check if already added (simple check by searching /etc/apt/sources.list.d/)
	ppaSlug := strings.TrimPrefix(ppa, "ppa:")
	ppaSlug = strings.ReplaceAll(ppaSlug, "/", "-")
	matches, _ := filepath.Glob("/etc/apt/sources.list.d/" + ppaSlug + "*.list")
	if len(matches) > 0 {
		utils.LogInfo(writer, "PPA %s already exists, skipping...", ppa)
		return nil
	}

	utils.LogInfo(writer, "Adding PPA: %s", ppa)
	cmd := exec.CommandContext(ctx, "sudo", "add-apt-repository", "-y", ppa)
	if err := runCommandWithColor(ctx, cmd, writer); err != nil {
		return fmt.Errorf("failed to add ppa %s: %v", ppa, err)
	}

	utils.LogInfo(writer, "Updating apt package list...")
	updateCmd := exec.CommandContext(ctx, "sudo", "apt-get", "update")
	if err := runCommandWithColor(ctx, updateCmd, writer); err != nil {
		return fmt.Errorf("failed to update apt after adding ppa: %v", err)
	}

	return nil
}

func LoadGlobalRecipe(libName string) (*types.DependencyRecipe, error) {
	recipePath := filepath.Join(utils.GetFinsHome(), "recipes.yaml")

	var config struct {
		Recipes map[string]types.DependencyRecipe `yaml:"recipes"`
	}
	data, err := os.ReadFile(recipePath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if r, ok := config.Recipes[libName]; ok {
		return &r, nil
	}
	return nil, fmt.Errorf("recipe for %s not found", libName)
}

func BuildDependency(ctx context.Context, libName, version string, recipe *types.DependencyRecipe, writer io.Writer, clearCache bool) error {
	root := GetDepRoot()
	installPath := filepath.Join(root, "install", libName, version)
	sourceDir := filepath.Join(root, "sources", libName, version)
	buildDir := filepath.Join(root, "build", libName, version)

	if clearCache {
		utils.LogSection(writer, "Clearing build cache for %s/%s", libName, version)
		os.RemoveAll(buildDir)
		os.RemoveAll(installPath)
	} else {
		if _, err := os.Stat(filepath.Join(installPath, "include")); err == nil {
			utils.LogSuccess(writer, "Dependency %s/%s already installed", libName, version)
			return nil
		}
	}

	targetGitURL := recipe.GitURL
	targetTag := version

	var versionSpecificArgs []string
	if v, ok := recipe.Versions[version]; ok {
		if v.GitURL != "" {
			targetGitURL = v.GitURL
		}
		if v.Tag != "" {
			targetTag = v.Tag
		}
		if len(v.CMakeArgs) > 0 {
			versionSpecificArgs = v.CMakeArgs
		}
	}

	if targetGitURL == "" {
		return fmt.Errorf("no git url found for %s", libName)
	}

	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("failed to create build dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(sourceDir), 0755); err != nil {
		return fmt.Errorf("failed to create source root: %v", err)
	}

	buildWriter := utils.NewBuildWriter(writer)

	if _, err := os.Stat(filepath.Join(sourceDir, ".git")); os.IsNotExist(err) {
		utils.LogSection(writer, "Cloning %s (%s)", libName, version)
		utils.LogInfo(writer, "Repo: %s", targetGitURL)
		cmd := exec.CommandContext(ctx, "git", "clone", "--recursive", targetGitURL, sourceDir)
		if err := runCommandWithColor(ctx, cmd, buildWriter); err != nil {
			os.RemoveAll(sourceDir)
			return fmt.Errorf("git clone failed: %v", err)
		}
	}

	utils.LogSection(writer, "Checking out ref: %s", targetTag)
	cmdCheckout := exec.CommandContext(ctx, "git", "checkout", targetTag)
	cmdCheckout.Dir = sourceDir
	runCommandWithColor(ctx, cmdCheckout, buildWriter)

	globalInstallRoot := filepath.Join(root, "install")
	args := []string{
		"-S", sourceDir,
		"-B", buildDir,
		"-DCMAKE_INSTALL_PREFIX=" + installPath,
		"-DCMAKE_PREFIX_PATH=" + globalInstallRoot,
		"-DCMAKE_BUILD_TYPE=Release",
	}

	args = append(args, recipe.CMakeArgs...)
	if len(versionSpecificArgs) > 0 {
		utils.LogSection(writer, "Applying version-specific CMake args for %s %s", libName, version)
		args = append(args, versionSpecificArgs...)
	}

	utils.LogSection(writer, "Configuring %s %s", libName, version)
	if err := runCommandWithColor(ctx, exec.CommandContext(ctx, "cmake", args...), buildWriter); err != nil {
		return err
	}

	utils.LogSection(writer, "Installing %s %s", libName, version)
	installArgs := []string{"--build", buildDir, "--target", "install", "-j8"}
	if err := runCommandWithColor(ctx, exec.CommandContext(ctx, "cmake", installArgs...), buildWriter); err != nil {
		return err
	}

	utils.LogSuccess(writer, "Dependency %s %s installed successfully", libName, version)
	return nil
}

func SolveDependencies(ctx context.Context, pkg *types.Package, writer io.Writer, clearCache bool) error {
	utils.LogSection(writer, "Solving dependencies for %s", pkg.Meta.Name)

	rosDistro := utils.GetROSDistro()

	for lib, ver := range pkg.Meta.Depends {
		var activeRecipe *types.DependencyRecipe

		if localRecipe, ok := pkg.Meta.Recipes[lib]; ok {
			utils.LogSection(writer, "Using local recipe for %s (defined in package.yaml)", lib)
			activeRecipe = &localRecipe
		} else {
			globalRecipe, err := LoadGlobalRecipe(lib)
			if err != nil {
				utils.LogError(writer, "Failed to load recipe for %s: %v", lib, err)
				return fmt.Errorf("failed to load recipe for %s: %v", lib, err)
			}
			activeRecipe = globalRecipe
		}

		// Handle PPA if present
		if activeRecipe.PPA != "" {
			if err := handlePPA(ctx, activeRecipe.PPA, writer); err != nil {
				utils.LogError(writer, "PPA handle failed: %v", err)
				return err
			}
		}

		if ver == "system" {
			sysPkg := activeRecipe.SystemPackage
			if sysPkg == "" {
				utils.LogWarning(writer, "Dependency %s set as system but no system_pkg defined", lib)
				continue
			}

			if strings.Contains(sysPkg, "${ROS_DISTRO}") {
				sysPkg = strings.ReplaceAll(sysPkg, "${ROS_DISTRO}", rosDistro)
			}

			utils.LogSection(writer, "Ensuring system package: %s", sysPkg)
			cmd := exec.CommandContext(ctx, "sudo", "apt-get", "install", "-y", sysPkg)
			if err := runCommandWithColor(ctx, cmd, writer); err != nil {
				utils.LogError(writer, "System package installation failed: %v", err)
				return err
			}
			continue
		}

		if err := BuildDependency(ctx, lib, ver, activeRecipe, writer, clearCache); err != nil {
			utils.LogError(writer, "Failed to solve dependency %s: %v", lib, err)
			return fmt.Errorf("failed to solve dependency %s: %v", lib, err)
		}
	}

	return nil
}
