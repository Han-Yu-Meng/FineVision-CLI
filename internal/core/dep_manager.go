package core

import (
	"context"
	"finsd/internal/types"
	"finsd/internal/utils"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DepDirName = "dependencies"
)

func GetDepRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fins", DepDirName)
}

func GetLogDir() string {
	return utils.GetLogDir()
}

func LoadGlobalRecipe(libName string) (*types.DependencyRecipe, error) {
	home, _ := os.UserHomeDir()
	recipePath := filepath.Join(home, ".fins", "recipes.yaml")

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

	// 创建缩进的 Writer 用于 Git 和 CMake 输出
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

	for lib, ver := range pkg.Meta.Depends {
		if ver == "system" {
			continue
		}

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

		if err := BuildDependency(ctx, lib, ver, activeRecipe, writer, clearCache); err != nil {
			utils.LogError(writer, "Failed to solve dependency %s: %v", lib, err)
			return fmt.Errorf("failed to solve dependency %s: %v", lib, err)
		}
	}

	return nil
}
