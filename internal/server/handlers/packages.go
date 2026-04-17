package handlers

import (
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"finsd/internal/core"
	"finsd/internal/types"
	"finsd/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func InstallPlugin(c *gin.Context) {
	var req struct {
		Repo string `json:"repo"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  c.Writer,
		flusher: flusher,
	}

	repo := req.Repo
	if strings.HasPrefix(repo, "https://github.com/") {
		repo = strings.TrimPrefix(repo, "https://github.com/")
	} else if strings.HasPrefix(repo, "git@github.com:") {
		repo = strings.TrimPrefix(repo, "git@github.com:")
	}
	repo = strings.TrimSuffix(repo, ".git")
	repo = strings.TrimRight(repo, "/")

	if !strings.Contains(repo, "/") {
		utils.LogError(mw, "Invalid repository format. Expected 'owner/repo' or GitHub URL")
		return
	}

	utils.LogSection(mw, "Fetching latest release for %s...", repo)
	release, err := getLatestRelease(repo)
	if err != nil {
		utils.LogError(mw, "Failed to get latest release: %v", err)
		return
	}

	assetURL, assetName, err := findMatchingAsset(release)
	if err != nil {
		utils.LogError(mw, "Failed to find a matching asset: %v", err)
		return
	}

	utils.LogInfo(mw, "Downloading %s...", assetName)
	tmpZip, err := downloadAsset(assetURL)
	if err != nil {
		utils.LogError(mw, "Failed to download asset: %v", err)
		return
	}
	defer os.Remove(tmpZip)

	utils.LogInfo(mw, "Installing plugin to ~/.fins/install/...")
	err = installFromZip(tmpZip, mw)
	if err != nil {
		utils.LogError(mw, "Failed to install plugin: %v", err)
		return
	}

	utils.LogSuccess(mw, "Plugin installed successfully from %s", repo)
}

func getLatestRelease(repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API Error: %s, Body: %s", resp.Status, string(body))
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func findMatchingAsset(release *GitHubRelease) (string, string, error) {
	osName := "Ubuntu-22.04"

	if content, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(content), "\n")
		var isUbuntu bool
		var version string
		for _, line := range lines {
			if strings.HasPrefix(line, "ID=ubuntu") {
				isUbuntu = true
			} else if strings.HasPrefix(line, "VERSION_ID=") {
				version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
			}
		}
		if isUbuntu && version != "" {
			osName = fmt.Sprintf("Ubuntu-%s", version)
		}
	}

	arch := runtime.GOARCH
	target := fmt.Sprintf("%s-%s.zip", osName, arch)

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, target) {
			return asset.BrowserDownloadURL, asset.Name, nil
		}
	}

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, arch) && strings.Contains(asset.Name, "Ubuntu") && strings.HasSuffix(asset.Name, ".zip") {
			return asset.BrowserDownloadURL, asset.Name, nil
		}
	}

	return "", "", fmt.Errorf("no matching asset found for %s or architecture %s", target, arch)
}

func downloadAsset(assetURL string) (string, error) {
	proxyPrefix := os.Getenv("FINS_GITHUB_PROXY")
	finalURL := assetURL
	if proxyPrefix != "" {
		finalURL = proxyPrefix + assetURL
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			log.Printf("Redirecting to: %s", req.URL.String())
			return nil
		},
	}

	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && proxyPrefix == "" {
		log.Println("Direct download failed with 403, retrying with proxy...")
		return downloadWithProxy(assetURL)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download from %s: %s", finalURL, resp.Status)
	}

	tmpFile, err := os.CreateTemp("", "fins-plugin-*.zip")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return "", err
	}

	return tmpFile.Name(), nil
}

func downloadWithProxy(assetURL string) (string, error) {
	proxy := "https://ghproxy.com/"
	finalURL := proxy + assetURL
	client := &http.Client{}
	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download with backup proxy: %s", resp.Status)
	}

	tmpFile, err := os.CreateTemp("", "fins-plugin-*.zip")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = io.Copy(tmpFile, resp.Body)
	return tmpFile.Name(), err
}

func installFromZip(zipPath string, w io.Writer) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %v", err)
	}
	installDir := filepath.Join(home, ".fins", "install")

	if _, err := os.Stat(installDir); os.IsNotExist(err) {
		err = os.MkdirAll(installDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create install directory %s: %v", installDir, err)
		}
	}

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, ".so") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			path := filepath.Join(installDir, filepath.Base(f.Name))
			dstFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer dstFile.Close()

			if _, err = io.Copy(dstFile, rc); err != nil {
				return err
			}
			utils.LogInfo(w, "Extracted %s to %s", f.Name, path)
		}
	}

	return nil
}

func GetPackages(c *gin.Context) {
	data := PackageWatcher.GetPackages()
	if strings.Contains(c.Request.Header.Get("Accept-Encoding"), "gzip") {
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(c.Writer)
		defer gz.Close()
		c.Header("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(gz).Encode(data)
	} else {
		c.JSON(200, data)
	}
}

func GetPackageDetail(c *gin.Context) {
	name := c.Param("name")
	if len(name) > 1 && name[0] == '/' {
		name = name[1:]
	}

	p := PackageWatcher.GetPackage(name)

	if p == nil {
		source := c.Query("source")
		if source != "" && source != "Unknown" {
			p = PackageWatcher.GetPackage(source + "/" + name)
		}
	}

	if p == nil {
		pkgs := PackageWatcher.GetPackages()
		for _, pkg := range pkgs {
			if strings.HasSuffix(pkg.Name, "/"+name) {
				source := c.Query("source")
				if source != "" && source != "Unknown" && pkg.Source != source {
					continue
				}
				realPkg := PackageWatcher.GetPackage(pkg.Name)
				if realPkg != nil {
					p = realPkg
					break
				}
			}
		}
	}

	if p == nil {
		c.JSON(404, gin.H{"error": "Package not found"})
		return
	}

	details := types.PackageDetails{
		Package:       *p,
		ReadmeContent: "",
	}

	if p.ReadmePath != "" {
		if content, err := os.ReadFile(p.ReadmePath); err == nil {
			details.ReadmeContent = string(content)
		}
	}

	c.JSON(200, details)
}

func GetPackageAsset(c *gin.Context) {
	fullPath := c.Param("path")
	if len(fullPath) > 1 && fullPath[0] == '/' {
		fullPath = fullPath[1:]
	}

	var matchedPkg *types.Package
	var relPath string

	pkgs := PackageWatcher.GetPackages()

	var pkgNames []string
	for _, p := range pkgs {
		pkgNames = append(pkgNames, p.Name)
	}
	sort.Slice(pkgNames, func(i, j int) bool { return len(pkgNames[i]) > len(pkgNames[j]) })

	for _, name := range pkgNames {
		if strings.HasPrefix(fullPath, name+"/") {
			matchedPkg = PackageWatcher.GetPackage(name)
			relPath = strings.TrimPrefix(fullPath, name+"/")
			break
		}
	}

	if matchedPkg == nil {
		parts := strings.SplitN(fullPath, "/", 2)
		if len(parts) >= 2 {
			potentialShortName := parts[0]
			relPathCandidate := parts[1]

			for _, p := range pkgs {
				if p.Name == potentialShortName || strings.HasSuffix(p.Name, "/"+potentialShortName) {
					matchedPkg = PackageWatcher.GetPackage(p.Name)
					relPath = relPathCandidate
					break
				}
			}
		}
	}

	if matchedPkg != nil {
		if strings.Contains(relPath, "..") {
			c.Status(403)
			return
		}

		file := filepath.Join(matchedPkg.Path, relPath)
		if _, err := os.Stat(file); err == nil {
			c.File(file)
		} else {
			c.Status(404)
		}
	} else {
		c.Status(404)
	}
}

func GetPackageLog(c *gin.Context) {
	name := c.Param("name")
	if len(name) > 1 && name[0] == '/' {
		name = name[1:]
	}

	safeName := strings.ReplaceAll(name, "/", "_")
	logPath := filepath.Join(core.GetLogDir(), safeName+".log")

	if content, err := os.ReadFile(logPath); err == nil {
		c.String(200, string(content))
	} else {
		c.String(200, "")
	}
}

func TriggerScan(c *gin.Context) {
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Failed to reload config during scan: %v", err)
	}
	PackageWatcher.Rescan()
	c.JSON(200, gin.H{"message": "Package scan triggered"})
}
