package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"finsd/internal/core"
	"finsd/internal/types"

	"github.com/gin-gonic/gin"
)

// CompilePackage 编译包
func CompilePackage(c *gin.Context) {
	pkgName := c.Param("name")
	if len(pkgName) > 1 && pkgName[0] == '/' {
		pkgName = pkgName[1:]
	}

	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	safeName := strings.ReplaceAll(pkgName, "/", "_")
	logPath := filepath.Join(core.GetLogDir(), safeName+".log")
	logFile, _ := os.Create(logPath)
	defer logFile.Close()

	baseMw := io.MultiWriter(logFile, c.Writer)
	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  baseMw,
		flusher: flusher,
	}

	PackageWatcher.UpdateStatus(pkgName, types.StatusCompiling)

	err := core.CompilePackageStream(c.Request.Context(), pkgName, mw)

	if err != nil {
		if c.Request.Context().Err() != nil {
			fmt.Fprintf(mw, "\n[INFO] Compilation cancelled by user\n")
			return
		}
		errMsg := fmt.Sprintf("\n[ERROR] Compilation Failed: %v\n", err)
		mw.Write([]byte(errMsg))
		PackageWatcher.UpdateStatus(pkgName, types.StatusFailed)
	} else {
		PackageWatcher.UpdateStatus(pkgName, types.StatusCurrent)
	}
}

// CleanBuilds 清理构建缓存
func CleanBuilds(c *gin.Context) {
	if err := core.CleanAllBuilds(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
	} else {
		c.JSON(200, gin.H{"message": "Build cache cleaned"})
	}
}

// CompileAgent 编译 agent
func CompileAgent(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	logPath := filepath.Join(core.GetLogDir(), "agent_build.log")
	logFile, _ := os.Create(logPath)
	defer logFile.Close()

	baseMw := io.MultiWriter(logFile, c.Writer)
	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  baseMw,
		flusher: flusher,
	}

	if err := core.CompileAgent(c.Request.Context(), mw); err != nil {
		if c.Request.Context().Err() != nil {
			fmt.Fprintf(mw, "\n[INFO] Agent Compilation cancelled by user\n")
			return
		}
		mw.Write([]byte(fmt.Sprintf("\n[ERROR] Agent Compilation Failed: %v\n", err)))
	}
}

// CompileInspect 编译 inspect 工具
func CompileInspect(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")

	logPath := filepath.Join(core.GetLogDir(), "inspect_build.log")
	logFile, _ := os.Create(logPath)
	defer logFile.Close()

	baseMw := io.MultiWriter(logFile, c.Writer)
	flusher, _ := c.Writer.(http.Flusher)
	mw := &FlushableMultiWriter{
		Writer:  baseMw,
		flusher: flusher,
	}

	if err := core.CompileInspect(c.Request.Context(), mw); err != nil {
		if c.Request.Context().Err() != nil {
			fmt.Fprintf(mw, "\n[INFO] Inspect Compilation cancelled by user\n")
			return
		}
		mw.Write([]byte(fmt.Sprintf("\n[ERROR] Inspect Compilation Failed: %v\n", err)))
	}
}

// AnalyzePackage 分析包
func AnalyzePackage(c *gin.Context) {
	name := c.Param("name")
	if len(name) > 1 && name[0] == '/' {
		name = name[1:]
	}

	result, err := core.RunInspect(name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(404, gin.H{"error": err.Error()})
		} else {
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}

	c.Data(200, "application/json; charset=utf-8", []byte(result))
}

// AnalyzeFile 分析指定的 .so 文件
func AnalyzeFile(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(400, gin.H{"error": "path parameter is required"})
		return
	}

	result, err := core.RunInspectFile(path)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(404, gin.H{"error": err.Error()})
		} else {
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}

	c.Data(200, "application/json; charset=utf-8", []byte(result))
}
