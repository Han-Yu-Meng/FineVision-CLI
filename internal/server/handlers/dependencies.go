package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"finsd/internal/core"
	"finsd/internal/utils"

	"github.com/gin-gonic/gin"
)

func BuildDependency(c *gin.Context) {
	var req struct {
		Library string `json:"library"`
		Version string `json:"version"`
		Clear   bool   `json:"clear"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	safeVer := strings.ReplaceAll(req.Version, "/", "_")
	logPath := filepath.Join(core.GetLogDir(), fmt.Sprintf("dep_%s_%s.log", req.Library, safeVer))
	logFile, _ := os.Create(logPath)
	defer logFile.Close()

	baseMw := io.MultiWriter(logFile, c.Writer)
	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  baseMw,
		flusher: flusher,
	}

	utils.LogSection(mw, "Requesting build for dependency: %s = %s", req.Library, req.Version)

	recipe, err := core.LoadGlobalRecipe(req.Library)
	if err != nil {
		utils.LogError(mw, "Recipe not found: %v", err)
		return
	}

	if err := core.BuildDependency(c.Request.Context(), req.Library, req.Version, recipe, mw, req.Clear); err != nil {
		utils.LogError(mw, "Dependency build failed: %v", err)
	} else {
		utils.LogSuccess(mw, "Dependency ready.")
	}
}

func SolveDependencies(c *gin.Context) {
	pkgName := c.Param("name")
	if len(pkgName) > 1 && pkgName[0] == '/' {
		pkgName = pkgName[1:]
	}

	clearCache := c.Query("clear") == "true"

	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	logPath := filepath.Join(core.GetLogDir(), fmt.Sprintf("dep_solve_%s.log", strings.ReplaceAll(pkgName, "/", "_")))
	logFile, _ := os.Create(logPath)
	defer logFile.Close()

	baseMw := io.MultiWriter(logFile, c.Writer)
	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  baseMw,
		flusher: flusher,
	}

	p := PackageWatcher.GetPackage(pkgName)
	if p == nil {
		pkgs := PackageWatcher.GetPackages()
		for _, pkg := range pkgs {
			if strings.HasSuffix(pkg.Name, "/"+pkgName) || pkg.Name == pkgName {
				p = PackageWatcher.GetPackage(pkg.Name)
				break
			}
		}
	}

	if p == nil {
		utils.LogError(mw, "Package '%s' not found", pkgName)
		return
	}

	utils.LogSection(mw, "Solving dependencies for package: %s", p.Meta.Name)

	if err := core.SolveDependencies(c.Request.Context(), p, mw, clearCache); err != nil {
		utils.LogError(mw, "Solve Failed: %v", err)
	} else {
		utils.LogSuccess(mw, "All dependencies are ready.")
	}
}

func GetRecipe(c *gin.Context) {
	name := c.Param("name")
	recipe, err := core.LoadGlobalRecipe(name)
	if err != nil {
		c.JSON(404, gin.H{"error": "Recipe not found"})
		return
	}

	rosDistro := os.Getenv("ROS_DISTRO")
	if rosDistro == "" {
		distros := []string{"jazzy", "humble", "iron", "galactic", "foxy"}
		for _, d := range distros {
			if _, err := os.Stat("/opt/ros/" + d); err == nil {
				rosDistro = d
				break
			}
		}
		if rosDistro == "" {
			rosDistro = "humble"
		}
	}

	if strings.Contains(recipe.SystemPackage, "${ROS_DISTRO}") {
		recipe.SystemPackage = strings.ReplaceAll(recipe.SystemPackage, "${ROS_DISTRO}", rosDistro)
	}

	c.JSON(200, recipe)
}
