package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/fausto2022/relaydeck/backend/config"
	"github.com/gin-gonic/gin"
)

func registerAuth(g *gin.RouterGroup, d *Deps) {
	g.POST("/auth/login", func(c *gin.Context) { login(c, d) })
	g.POST("/auth/change-credentials", func(c *gin.Context) { changeCredentials(c, d) })
	g.GET("/auth/me", func(c *gin.Context) { whoami(c, d) })
	g.POST("/auth/logout", func(c *gin.Context) {
		// 无状态 token，客户端丢弃即可；这个接口仅作语义存在。
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}

type loginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type changeCredentialsInput struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	Username        string `json:"username" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

func login(c *gin.Context, d *Deps) {
	// 鉴权关闭：任何登录请求都直接成功；前端在 /auth/me 已经知道无需登录。
	authSvc := d.Runtime.CurrentAuth()
	if authSvc == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"auth_disabled": true,
				"username":      "anonymous",
			},
		})
		return
	}
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	token, exp, err := authSvc.Login(in.Username, in.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"token":      token,
			"expires_at": exp.Unix(),
			"username":   authSvc.Username(),
		},
	})
}

// whoami 既是"前端启动探测"接口也是"已登录信息"接口。
//
//   - 鉴权关闭 → 返回 {auth_disabled: true}，前端据此跳过登录页
//   - 鉴权开启但未带 token → 中间件已经在前面 401 拦截，根本走不到这里
//   - 鉴权开启 + 有效 token → 返回 username
func whoami(c *gin.Context, d *Deps) {
	if d.Runtime.CurrentAuth() == nil {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"auth_disabled": true,
				"username":      "anonymous",
			},
		})
		return
	}
	sub, _ := c.Get("authSubject")
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"username": sub}})
}

func changeCredentials(c *gin.Context, d *Deps) {
	authSvc := d.Runtime.CurrentAuth()
	if authSvc == nil {
		fail(c, http.StatusBadRequest, errors.New("当前未启用登录鉴权"))
		return
	}

	var in changeCredentialsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, errors.New("账号密码格式不正确"))
		return
	}
	if !authSvc.VerifyPassword(in.CurrentPassword) {
		fail(c, http.StatusUnauthorized, errors.New("当前密码错误"))
		return
	}

	username := strings.TrimSpace(in.Username)
	if username == "" {
		fail(c, http.StatusBadRequest, errors.New("管理员账号不能为空"))
		return
	}
	if utf8.RuneCountInString(in.NewPassword) < 8 {
		fail(c, http.StatusBadRequest, errors.New("新密码至少需要 8 位"))
		return
	}

	path := d.Runtime.ConfigPath()
	cfg, err := config.LoadFile(path)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if !cfg.Auth.Enabled {
		fail(c, http.StatusBadRequest, errors.New("当前未启用登录鉴权"))
		return
	}
	previousAuth := cfg.Auth
	cfg.Auth.Username = username
	cfg.Auth.Password = in.NewPassword
	if cfg.Auth.SessionVersion < 0 {
		cfg.Auth.SessionVersion = 0
	}
	cfg.Auth.SessionVersion++
	if err := config.Save(path, cfg); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := d.Runtime.ApplyFromFile(); err != nil {
		cfg.Auth = previousAuth
		if restoreErr := config.Save(path, cfg); restoreErr != nil && d.Log != nil {
			d.Log.Error("restore auth config after apply failure", "path", path, "err", restoreErr)
		}
		fail(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"username": username,
		"message":  "账号密码已更新，请重新登录",
	}})
}
