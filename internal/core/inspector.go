package core

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"finsd/internal/types"
	"finsd/internal/utils"

	"github.com/spf13/viper"
)

func resolvePackageForInspect(pkgName string) (*types.Package, error) {
	pkgs, err := ScanPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to scan packages: %v", err)
	}

	return ResolvePackage(pkgName, pkgs)
}

func RunInspect(pkgName string) (string, error) {
	targetPkg, err := resolvePackageForInspect(pkgName)
	if err != nil {
		return "", err
	}

	binDir := utils.ExpandPath(viper.GetString("build.defaults.build_output"))

	soName := fmt.Sprintf("lib%s_%s.so", targetPkg.Source, targetPkg.Meta.Name)
	soPath := filepath.Join(binDir, soName)

	return RunInspectFile(soPath)
}

func RunInspectFile(soPath string) (string, error) {
	binDir := utils.ExpandPath(viper.GetString("build.defaults.build_output"))
	inspectBin := filepath.Join(binDir, "inspect")

	actualPath := soPath
	if !filepath.IsAbs(soPath) {
		actualPath = filepath.Join(binDir, soPath)
	}

	if _, err := os.Stat(inspectBin); os.IsNotExist(err) {
		return "", fmt.Errorf("inspect tool not found at %s. Please run 'fins inspect build' first", inspectBin)
	}

	var targetPath string
	if strings.ContainsAny(actualPath, "*?[]") {
		matches, err := filepath.Glob(actualPath)
		if err != nil {
			return "", fmt.Errorf("failed to process glob pattern: %v", err)
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("binary file not found at %s", actualPath)
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("ambiguous file pattern: %d files match: %v", len(matches), matches)
		}
		targetPath = matches[0]
	} else {
		targetPath = actualPath
	}

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return "", fmt.Errorf("binary file not found at %s", targetPath)
	}

	cmd := exec.Command(inspectBin, targetPath)

	soDir := filepath.Dir(targetPath)
	env := os.Environ()
	found := false
	for i, v := range env {
		if strings.HasPrefix(v, "LD_LIBRARY_PATH=") {
			env[i] = v + ":" + soDir
			found = true
			break
		}
	}
	if !found {
		env = append(env, "LD_LIBRARY_PATH="+soDir)
	}
	cmd.Env = env

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("inspect execution failed: %v, stderr: %s", err, stderr.String())
	}

	return out.String(), nil
}
