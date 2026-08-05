package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const clientErrorBodyLimit int64 = 64 << 10

type clientErrorInput struct {
	Name                string `json:"name"`
	Message             string `json:"message"`
	Stack               string `json:"stack"`
	ComponentStack      string `json:"component_stack"`
	URL                 string `json:"url"`
	UserAgent           string `json:"user_agent"`
	DocumentLanguage    string `json:"document_language"`
	DOMMismatch         bool   `json:"dom_mismatch"`
	TranslationDetected bool   `json:"translation_detected"`
}

func registerClientErrors(g *gin.RouterGroup, d *Deps) {
	g.POST("/client-errors", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, clientErrorBodyLimit)
		var in clientErrorInput
		if err := c.ShouldBindJSON(&in); err != nil {
			fail(c, http.StatusBadRequest, errors.New("前端错误日志格式不正确"))
			return
		}
		in.Message = clientErrorField(in.Message, 2_000)
		if in.Message == "" {
			fail(c, http.StatusBadRequest, errors.New("前端错误信息不能为空"))
			return
		}
		if d.Log != nil {
			d.Log.Error("frontend application error",
				"name", clientErrorField(in.Name, 128),
				"message", in.Message,
				"stack", clientErrorField(in.Stack, 16_000),
				"component_stack", clientErrorField(in.ComponentStack, 16_000),
				"url", clientErrorField(in.URL, 2_000),
				"user_agent", clientErrorField(in.UserAgent, 1_000),
				"document_language", clientErrorField(in.DocumentLanguage, 32),
				"dom_mismatch", in.DOMMismatch,
				"translation_detected", in.TranslationDetected,
			)
		}
		c.Status(http.StatusNoContent)
	})
}

func clientErrorField(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
