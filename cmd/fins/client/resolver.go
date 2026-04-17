package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"fins-cli/internal/types"
	"fins-cli/internal/utils"
)

func ResolvePackageIdentity(daemonURL, inputName, targetSource string) (string, error) {
	if strings.Contains(inputName, "/") {
		if targetSource != "" {
			parts := strings.Split(inputName, "/")
			if parts[0] != targetSource {
				return "", fmt.Errorf("package '%s' does not match requested source '%s'", inputName, targetSource)
			}
		}
		return inputName, nil
	}

	pkgs, err := FetchPackageList(daemonURL)
	if err != nil {
		return "", err
	}

	var candidates []types.PackageInfo
	for _, p := range pkgs {
		parts := strings.Split(p.Name, "/")
		shortName := parts[len(parts)-1]

		if shortName == inputName {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("package '%s' not found", inputName)
	}

	if len(candidates) == 1 {
		found := candidates[0]
		if targetSource != "" {
			parts := strings.Split(found.Name, "/")
			if parts[0] != targetSource {
				return "", fmt.Errorf("package '%s' found in '%s', but you requested source '%s'", inputName, parts[0], targetSource)
			}
		}
		return found.Name, nil
	}

	if targetSource != "" {
		for _, c := range candidates {
			parts := strings.Split(c.Name, "/")
			if parts[0] == targetSource {
				return c.Name, nil
			}
		}
		return "", fmt.Errorf("package '%s' exists, but not in source '%s'", inputName, targetSource)
	}

	var sources []string
	for _, c := range candidates {
		parts := strings.Split(c.Name, "/")
		sources = append(sources, parts[0])
	}

	utils.LogError(os.Stdout, "Ambiguous package name '%s'. Found in sources: %s.", inputName, strings.Join(sources, ", "))
	return "", fmt.Errorf("use --source <source> to specify which package to use")
}

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
