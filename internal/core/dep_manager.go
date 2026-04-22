package core

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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

func calculateArgsHash(args []string) string {
	if len(args) == 0 {
		return "default"
	}
	h := sha1.New()
	h.Write([]byte(strings.Join(args, "|")))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

func GetDependencyPaths(libName, version string, recipe *types.DependencyRecipe) (install, source, build string, hash string) {
	root := GetDepRoot()

	allArgs := append([]string{}, recipe.CMakeArgs...)
	if v, ok := recipe.Versions[version]; ok {
		allArgs = append(allArgs, v.CMakeArgs...)
	}

	hash = calculateArgsHash(allArgs)
	verHash := fmt.Sprintf("%s-%s", version, hash)

	install = filepath.Join(root, "install", libName, verHash)
	source = filepath.Join(root, "sources", libName, version)
	build = filepath.Join(root, "build", libName, verHash)
	return
}

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

	ppaSlug := strings.TrimPrefix(ppa, "ppa:")
	ppaSlug = strings.ReplaceAll(ppaSlug, "/", "-")
	matches, _ := filepath.Glob("/etc/apt/sources.list.d/" + ppaSlug + "*.list")
	if len(matches) > 0 {
		utils.LogInfo(writer, "PPA %s already exists.", ppa)
		return nil
	}

	utils.LogWarning(writer, "PPA %s seems NOT added. Please run 'sudo fins dep install' to add it.", ppa)
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
	// Get hash-based paths
	installPath, sourceDir, buildDir, _ := GetDependencyPaths(libName, version, recipe)

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

	if v, ok := recipe.Versions[version]; ok {
		if v.GitURL != "" {
			targetGitURL = v.GitURL
		}
		if v.Tag != "" {
			targetTag = v.Tag
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
		utils.LogSection(writer, "Cloning %s (Tag: %s) [Shallow]", libName, targetTag)

		cloneArgs := []string{
			"clone",
			"--depth", "1",
			"--branch", targetTag,
			"--recursive",
			"--shallow-submodules",
			targetGitURL,
			sourceDir,
		}

		cmd := exec.CommandContext(ctx, "git", cloneArgs...)
		if err := runCommandWithColor(ctx, cmd, buildWriter); err != nil {
			os.RemoveAll(sourceDir)
			return fmt.Errorf("git shallow clone failed: %v", err)
		}
	} else {
		// If source code already exists, ensure it's the correct Tag
		utils.LogInfo(writer, "Source exists, ensuring ref: %s", targetTag)
		cmdCheckout := exec.CommandContext(ctx, "git", "checkout", targetTag)
		cmdCheckout.Dir = sourceDir
		runCommandWithColor(ctx, cmdCheckout, buildWriter)
	}

	globalInstallRoot := filepath.Join(GetDepRoot(), "install")
	args := []string{
		"-S", sourceDir,
		"-B", buildDir,
		"-DCMAKE_INSTALL_PREFIX=" + installPath,
		"-DCMAKE_PREFIX_PATH=" + globalInstallRoot,
		"-DCMAKE_BUILD_TYPE=Release",
	}

	args = append(args, recipe.CMakeArgs...)
	if v, ok := recipe.Versions[version]; ok {
		args = append(args, v.CMakeArgs...)
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
				utils.LogWarning(writer, "Recipe for '%s' not found: %v.", lib, err)
				continue
			}
			activeRecipe = globalRecipe
		}

		// Handle PPA if present
		if activeRecipe != nil && activeRecipe.PPA != "" {
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

			pkgs := strings.Fields(sysPkg)
			allInstalled := true
			for _, p := range pkgs {
				checkCmd := exec.CommandContext(ctx, "dpkg", "-s", p)
				if err := checkCmd.Run(); err != nil {
					utils.LogWarning(writer, "System package '%s' (for %s) seems NOT installed. Build might fail.", p, lib)
					allInstalled = false
				}
			}
			if allInstalled {
				utils.LogInfo(writer, "System package for '%s' are already installed.", lib)
			} else {
				utils.LogWarning(writer, "Please run 'sudo fins dep install %s' to fix missing system dependencies.", pkg.Meta.Name)
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
