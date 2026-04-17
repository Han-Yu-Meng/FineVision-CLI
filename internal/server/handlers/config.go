package handlers

import (
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func GetPresets(c *gin.Context) {
	current := viper.GetString("build.default_preset")
	presetsMap := viper.GetStringMap("build.presets")

	var names []string
	for k := range presetsMap {
		names = append(names, k)
	}
	sort.Strings(names)

	c.JSON(200, gin.H{
		"current": current,
		"presets": names,
	})
}

func SetPreset(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.Status(400)
		return
	}

	presets := viper.GetStringMap("build.presets")
	if _, ok := presets[req.Name]; !ok {
		c.JSON(404, gin.H{"error": "Preset not found"})
		return
	}

	viper.Set("build.default_preset", req.Name)
	viper.WriteConfig()

	c.Status(200)
}
