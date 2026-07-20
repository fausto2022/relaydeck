package api

import (
	"errors"
	"net/http"

	"github.com/fausto2022/relaydeck/backend/config"
	"github.com/fausto2022/relaydeck/backend/rateranking"
	"github.com/gin-gonic/gin"
)

type settingsConfigView struct {
	Auth          config.AuthConfig          `json:"auth"`
	Scheduler     config.SchedulerConfig     `json:"scheduler"`
	Notifications config.NotificationsConfig `json:"notifications"`
	Proxy         config.ProxyConfig         `json:"proxy"`
	Upstream      config.UpstreamConfig      `json:"upstream"`
}

type settingsConfigInput struct {
	Auth          config.AuthConfig          `json:"auth" binding:"required"`
	Scheduler     config.SchedulerConfig     `json:"scheduler" binding:"required"`
	Notifications config.NotificationsConfig `json:"notifications" binding:"required"`
	Proxy         config.ProxyConfig         `json:"proxy"`
	Upstream      config.UpstreamConfig      `json:"upstream"`
}

func registerSettings(g *gin.RouterGroup, d *Deps) {
	gs := g.Group("/settings")
	gs.GET("/config", func(c *gin.Context) { getSettingsConfig(c, d) })
	gs.PUT("/config", func(c *gin.Context) { saveSettingsConfig(c, d) })
	gs.POST("/apply", func(c *gin.Context) { applySettingsConfig(c, d) })
	gs.POST("/proxy/test", func(c *gin.Context) { testProxy(c) })
	gs.GET("/rate-ranking", func(c *gin.Context) { getRateRankingConfig(c, d) })
	gs.PUT("/rate-ranking", func(c *gin.Context) { saveRateRankingConfig(c, d) })
}

func getRateRankingConfig(c *gin.Context, d *Deps) {
	if d.RateRanking == nil {
		fail(c, http.StatusServiceUnavailable, errors.New("倍率排行分类服务尚未初始化"))
		return
	}
	config, err := d.RateRanking.Get(c.Request.Context())
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": config})
}

func saveRateRankingConfig(c *gin.Context, d *Deps) {
	if d.RateRanking == nil {
		fail(c, http.StatusServiceUnavailable, errors.New("倍率排行分类服务尚未初始化"))
		return
	}
	var input rateranking.Config
	if err := c.ShouldBindJSON(&input); err != nil {
		fail(c, http.StatusBadRequest, errors.New("倍率排行分类配置格式不正确"))
		return
	}
	config, err := d.RateRanking.Save(c.Request.Context(), input)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": config})
}

func getSettingsConfig(c *gin.Context, d *Deps) {
	cfg, err := config.LoadFile(d.Runtime.ConfigPath())
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"config_path": d.Runtime.ConfigPath(),
			"config": settingsConfigView{
				Auth:          cfg.Auth,
				Scheduler:     cfg.Scheduler,
				Notifications: cfg.Notifications,
				Proxy:         cfg.Proxy,
				Upstream:      cfg.Upstream,
			},
		},
	})
}

func saveSettingsConfig(c *gin.Context, d *Deps) {
	var in settingsConfigInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}

	path := d.Runtime.ConfigPath()
	cfg, err := config.LoadFile(path)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	cfg.Auth = in.Auth
	cfg.Scheduler = in.Scheduler
	cfg.Notifications = in.Notifications
	cfg.Proxy = in.Proxy
	cfg.Upstream = in.Upstream.WithDefaults()

	if err := config.Save(path, cfg); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"config_path": path,
			"message":     "已写入配置文件",
		},
	})
}

func applySettingsConfig(c *gin.Context, d *Deps) {
	result, err := d.Runtime.ApplyFromFile()
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
