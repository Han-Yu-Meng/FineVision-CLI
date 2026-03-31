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

// resolvePackageForInspect 解析包名，类似于 client resolver 但用于服务端
func resolvePackageForInspect(pkgName string) (*types.Package, error) {
	pkgs, err := ScanPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to scan packages: %v", err)
	}

	// 1. 如果包含 /，认为是全名 (Source/Name)
	if strings.Contains(pkgName, "/") {
		if p, ok := pkgs[pkgName]; ok {
			return p, nil
		}
		return nil, fmt.Errorf("package '%s' not found", pkgName)
	}

	// 2. 短名匹配，筛选候选者
	var candidates []*types.Package
	for _, p := range pkgs {
		if p.Meta.Name == pkgName {
			candidates = append(candidates, p)
		}
	}

	// 3. 决策逻辑
	if len(candidates) == 0 {
		return nil, fmt.Errorf("package '%s' not found", pkgName)
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	// 4. 存在歧义
	var sources []string
	for _, c := range candidates {
		sources = append(sources, c.Source)
	}
	return nil, fmt.Errorf("ambiguous package name '%s'. Found in sources: %s. Please use 'Source/Name' format", pkgName, strings.Join(sources, ", "))
}

// RunInspect 执行 inspect 二进制文件分析指定包生成的 .so 文件
func RunInspect(pkgName string) (string, error) {
	// 1. 获取构建输出目录
	binDir := utils.ExpandPath(viper.GetString("build.defaults.build_output"))
	inspectBin := filepath.Join(binDir, "inspect")

	// 2. 检查 inspect 工具是否存在
	if _, err := os.Stat(inspectBin); os.IsNotExist(err) {
		return "", fmt.Errorf("inspect tool not found at %s. Please run 'fins inspect build' first", inspectBin)
	}

	// 3. 解析包名
	targetPkg, err := resolvePackageForInspect(pkgName)
	if err != nil {
		return "", err
	}

	// 4. 构造 .so 文件路径
	soName := fmt.Sprintf("lib%s_%s.so", targetPkg.Source, targetPkg.Meta.Name)
	soPath := filepath.Join(binDir, soName)

	if _, err := os.Stat(soPath); os.IsNotExist(err) {
		return "", fmt.Errorf("binary for package '%s' not found (%s). Has it been compiled?", pkgName, soName)
	}

	// 5. 执行 inspect 命令
	cmd := exec.Command(inspectBin, soPath)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("inspect execution failed: %v, stderr: %s", err, stderr.String())
	}

	return out.String(), nil
}
