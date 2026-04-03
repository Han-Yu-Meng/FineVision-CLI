package core

import (
	"finsd/internal/types"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func ScanPackages() (map[string]*types.Package, error) {
	pkgs := make(map[string]*types.Package)

	// 1. Scan Local Packages
	type LocalSource struct {
		Name string `mapstructure:"name"`
		Path string `mapstructure:"path"`
	}
	var localSources []LocalSource
	if err := viper.UnmarshalKey("local_packages", &localSources); err != nil {
		// handle error
	}

	// Fallback/Reinforcement: Manual parsing if UnmarshalKey failed to populate,
	// which can happen with viper on map slice structures sometimes
	if len(localSources) == 0 {
		if raw := viper.Get("local_packages"); raw != nil {
			if list, ok := raw.([]interface{}); ok {
				for _, item := range list {
					if m, ok := item.(map[string]interface{}); ok {
						name, _ := m["name"].(string)
						path, _ := m["path"].(string)
						// Retry with case insensitive lookup if empty
						if name == "" {
							name, _ = m["Name"].(string)
						}
						if path == "" {
							path, _ = m["Path"].(string)
						}

						if name != "" && path != "" {
							localSources = append(localSources, LocalSource{Name: name, Path: path})
						}
					}
				}
			}
		}
	}

	rawPkgs := make(map[string][]*types.Package)

	for _, src := range localSources {
		root := src.Path
		sourceName := src.Name

		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			// Don't scan into hidden directories
			if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
				return filepath.SkipDir
			}

			// Don't scan into known build/temp directories
			if d.IsDir() && (d.Name() == "build" || d.Name() == "devel" || d.Name() == "install") {
				return filepath.SkipDir
			}

			if !d.IsDir() && d.Name() == "package.yaml" {
				pkgPath := filepath.Dir(path)
				pkg := LoadPackage(pkgPath, path)
				if pkg != nil {
					pkg.Source = sourceName
					// Store raw package with just the short name first
					rawPkgs[pkg.Meta.Name] = append(rawPkgs[pkg.Meta.Name], pkg)
				}
				// Stop scanning deeper once a package.yaml is found in this directory
				return filepath.SkipDir
			}
			return nil
		})
	}

	// 3. Disambiguation
	for name, entryList := range rawPkgs {
		for _, p := range entryList {
			fullName := fmt.Sprintf("%s/%s", p.Source, name)
			pkgs[fullName] = p
		}
	}

	return pkgs, nil
}

func LoadPackage(path, metaPath string) *types.Package {
	var config struct {
		Package types.PackageMetadata `yaml:"package"`
	}

	data, _ := os.ReadFile(metaPath)
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil
	}

	if config.Package.Name == "" {
		_ = yaml.Unmarshal(data, &config.Package)
	}

	if config.Package.Name == "" {
		return nil
	}

	p := &types.Package{
		Path: path,
		Meta: config.Package,
	}

	if _, err := os.Stat(filepath.Join(path, "README.md")); err == nil {
		p.ReadmePath = filepath.Join(path, "README.md")
	}
	if _, err := os.Stat(filepath.Join(path, "install_deps.sh")); err == nil {
		p.ScriptPath = filepath.Join(path, "install_deps.sh")
	}

	if _, err := os.Stat(filepath.Join(path, "assets", "logo.png")); err == nil {
		p.IconPath = "assets/logo.png"
	} else if _, err := os.Stat(filepath.Join(path, "assets", "logo.jpg")); err == nil {
		p.IconPath = "assets/logo.jpg"
	} else if _, err := os.Stat(filepath.Join(path, "logo.png")); err == nil {
		p.IconPath = "logo.png"
	}

	p.Status = checkBuildStatus(p)
	return p
}

func checkBuildStatus(p *types.Package) types.BuildStatus {
	binDir := viper.GetString("build_output")
	// Must match the naming convention in builder.go (Source_Name)
	soName := fmt.Sprintf("lib%s_%s.so", p.Source, p.Meta.Name)
	soPath := filepath.Join(binDir, soName)

	soInfo, err := os.Stat(soPath)
	if os.IsNotExist(err) {
		return types.StatusUncompiled
	}

	isStale := false
	filepath.Walk(p.Path, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && (strings.HasSuffix(path, ".cpp") || strings.HasSuffix(path, ".hpp")) {
			if info.ModTime().After(soInfo.ModTime()) {
				isStale = true
				return filepath.SkipAll
			}
		}
		return nil
	})

	if isStale {
		return types.StatusStale
	}
	return types.StatusCurrent
}
