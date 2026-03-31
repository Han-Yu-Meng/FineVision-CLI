package handlers

import (
	"fmt"
	"os"
	"path/filepath"

	"finsd/internal/agent"
	"finsd/internal/core"

	"github.com/gin-gonic/gin"
)

// StartAgent 启动 agent
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

	if err := agent.GlobalManager.Start(req); err != nil {
		c.JSON(500, gin.H{"error": "Failed to start agent: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": fmt.Sprintf("Agent '%s' started successfully", req.AgentName)})
}

// StopAgent 停止 agent
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

// GetAgentStatus 获取 agent 状态
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
		// Return all statuses
		statuses := agent.GlobalManager.GetAllStatus()
		c.JSON(200, statuses)
	}
}

// GetAgentLogs 获取 agent 日志
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
