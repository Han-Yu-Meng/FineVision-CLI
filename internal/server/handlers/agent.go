package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"fins-cli/internal/agent"
	"fins-cli/internal/core"
	"fins-cli/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func StartAgent(c *gin.Context) {
	var req agent.AgentConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid configuration: " + err.Error()})
		return
	}

	if req.AgentName == "" {
		c.JSON(400, gin.H{"error": "Agent name is required"})
		return
	}

	if err := agent.GlobalManager.Start(req, false, nil); err != nil {
		c.JSON(500, gin.H{"error": "Failed to start agent: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": fmt.Sprintf("Agent '%s' started successfully", req.AgentName)})
}

func RunAgent(c *gin.Context) {
	var req agent.AgentConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid configuration: " + err.Error()})
		return
	}

	mw, flusher := InitStreamResponse(c)

	binDir := viper.GetString("build.defaults.build_output")
	agentBin := utils.ExpandPath(filepath.Join(binDir, "agent"))
	if _, err := os.Stat(agentBin); os.IsNotExist(err) {
		utils.LogSection(mw, "Agent binary not found, starting compilation...")
		if err := core.CompileAgent(c.Request.Context(), mw); err != nil {
			utils.LogError(mw, "Failed to compile agent: %v", err)
			return
		}
	}

	pr, pw, _ := os.Pipe()
	defer pr.Close()

	if err := agent.GlobalManager.Start(req, false, pw); err != nil {
		pw.Close()
		fmt.Fprintf(c.Writer, "Error: %v\n", err)
		flusher.Flush()
		return
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				c.Writer.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	}()

	notify := c.Request.Context().Done()
	agentDone := make(chan struct{})
	go func() {
		for {
			running, _, _ := agent.GlobalManager.GetStatus(req.AgentName)
			if !running {
				close(agentDone)
				return
			}
			select {
			case <-notify:
				agent.GlobalManager.Stop(req.AgentName)
				close(agentDone)
				return
			default:
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	<-agentDone
}

func DebugAgent(c *gin.Context) {
	var req agent.AgentConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid configuration: " + err.Error()})
		return
	}

	mw, flusher := InitStreamResponse(c)

	binDir := viper.GetString("build.defaults.build_output")
	agentBin := utils.ExpandPath(filepath.Join(binDir, "agent"))
	if _, err := os.Stat(agentBin); os.IsNotExist(err) {
		utils.LogSection(mw, "Agent binary not found, starting compilation...")
		if err := core.CompileAgent(c.Request.Context(), mw); err != nil {
			utils.LogError(mw, "Failed to compile agent: %v", err)
			return
		}
	}

	pr, pw, _ := os.Pipe()
	defer pr.Close()

	if err := agent.GlobalManager.Start(req, true, pw); err != nil {
		pw.Close()
		fmt.Fprintf(c.Writer, "Error: %v\n", err)
		flusher.Flush()
		return
	}

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				c.Writer.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	}()

	notify := c.Request.Context().Done()
	agentDone := make(chan struct{})
	go func() {
		for {
			running, _, _ := agent.GlobalManager.GetStatus(req.AgentName)
			if !running {
				close(agentDone)
				return
			}
			select {
			case <-notify:
				agent.GlobalManager.Stop(req.AgentName)
				close(agentDone)
				return
			default:
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()

	<-agentDone
}

func StopAgent(c *gin.Context) {
	var req struct {
		AgentName string `json:"agent_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	if req.AgentName == "" {
		c.JSON(400, gin.H{"error": "agent_name is required"})
		return
	}

	if err := agent.GlobalManager.Stop(req.AgentName); err != nil {
		c.JSON(500, gin.H{"error": "Failed to stop agent: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": fmt.Sprintf("Agent '%s' stopped", req.AgentName)})
}

func GetAgentStatus(c *gin.Context) {
	name := c.Query("name")
	if name != "" {
		running, pid, err := agent.GlobalManager.GetStatus(name)
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{
			"name":    name,
			"running": running,
			"pid":     pid,
		})
	} else {
		statuses := agent.GlobalManager.GetAllStatus()
		c.JSON(200, statuses)
	}
}

func GetAgentLogs(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		c.JSON(400, gin.H{"error": "name query parameter is required"})
		return
	}

	logPath := filepath.Join(core.GetLogDir(), fmt.Sprintf("agent_%s.log", name))
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		c.String(404, "No logs available for agent: "+name)
		return
	}
	c.File(logPath)
}
