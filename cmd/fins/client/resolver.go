package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"finsd/internal/types"
	"finsd/internal/utils"
)

// ResolvePackageIdentity 解析用户输入的包名，返回唯一的 "Source/Name" 格式
// daemonURL: 服务端地址
// inputName: 用户输入的包名 (可能是 short name 也可能是 full name)
// targetSource: 用户通过 --source 指定的源 (可选)
func ResolvePackageIdentity(daemonURL, inputName, targetSource string) (string, error) {
	// 1. 如果用户已经输入了全名 (包含 /)，直接返回
	if strings.Contains(inputName, "/") {
		// 简单的校验：如果有 targetSource，检查是否匹配
		if targetSource != "" {
			parts := strings.Split(inputName, "/")
			if parts[0] != targetSource {
				return "", fmt.Errorf("package '%s' does not match requested source '%s'", inputName, targetSource)
			}
		}
		return inputName, nil
	}

	// 2. 获取所有包列表
	pkgs, err := FetchPackageList(daemonURL)
	if err != nil {
		return "", err
	}

	// 3. 筛选候选者
	var candidates []types.PackageInfo
	for _, p := range pkgs {
		parts := strings.Split(p.Name, "/")
		shortName := parts[len(parts)-1]

		if shortName == inputName {
			candidates = append(candidates, p)
		}
	}

	// 4. 决策逻辑
	if len(candidates) == 0 {
		return "", fmt.Errorf("package '%s' not found", inputName)
	}

	if len(candidates) == 1 {
		// 唯一匹配，检查 targetSource 是否冲突
		found := candidates[0]
		if targetSource != "" {
			parts := strings.Split(found.Name, "/")
			if parts[0] != targetSource {
				return "", fmt.Errorf("package '%s' found in '%s', but you requested source '%s'", inputName, parts[0], targetSource)
			}
		}
		return found.Name, nil
	}

	// 5. 存在歧义 (Ambiguous)
	if targetSource != "" {
		// 尝试用 source 过滤
		for _, c := range candidates {
			parts := strings.Split(c.Name, "/")
			if parts[0] == targetSource {
				return c.Name, nil
			}
		}
		return "", fmt.Errorf("package '%s' exists, but not in source '%s'", inputName, targetSource)
	}

	// 构造错误信息，列出所有源
	var sources []string
	for _, c := range candidates {
		parts := strings.Split(c.Name, "/")
		sources = append(sources, parts[0])
	}

	utils.LogError(os.Stdout, "Ambiguous package name '%s'. Found in sources: %s.", inputName, strings.Join(sources, ", "))
	return "", fmt.Errorf("use --source <source> to specify which package to use")
}

// FetchPackageList 封装获取列表的请求
func FetchPackageList(daemonURL string) ([]types.PackageInfo, error) {
	resp, err := http.Get(daemonURL + "/api/packages")
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %v", err)
	}
	defer resp.Body.Close()

	var pkgs []types.PackageInfo
	if err := json.NewDecoder(resp.Body).Decode(&pkgs); err != nil {
		return nil, fmt.Errorf("failed to decode package list: %v", err)
	}
	return pkgs, nil
}
