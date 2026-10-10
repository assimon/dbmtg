package server

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/assimon/dbmtg/internal/service"
	"github.com/assimon/dbmtg/internal/web"
	"github.com/assimon/dbmtg/pkg/tginit"
)

// initData 有效期：从打开验证页到提交的最长时间
const initDataMaxAge = time.Hour

func (s *Server) verifyPage(c *gin.Context) {
	data := struct {
		Widget  template.HTML
		APIPath string
	}{APIPath: apiVerifyPath}
	if p := s.svc.Settings.CaptchaProvider(); p != nil {
		data.Widget = p.Widget()
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := web.Templates.ExecuteTemplate(c.Writer, "verify.html", data); err != nil {
		slog.Error("render verify page", "err", err)
	}
}

func (s *Server) verifyAPI(c *gin.Context) {
	var req struct {
		InitData string `json:"init_data"`
		Token    string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	u, err := tginit.Validate(req.InitData, s.cfg.Bot.Token, initDataMaxAge, time.Now())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请在 Telegram 中重新打开验证页面"})
		return
	}

	err = s.svc.Verify.Verify(c.Request.Context(), u.ID, req.Token, c.ClientIP())
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	case errors.Is(err, service.ErrAlreadyVerified):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "verified": true})
	case errors.Is(err, service.ErrBanned):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrCaptchaService):
		slog.Error("captcha", "err", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": service.ErrCaptchaService.Error()})
	case errors.Is(err, service.ErrVerifyFailed), errors.Is(err, service.ErrUserNotFound), errors.Is(err, service.ErrCaptchaDisabled):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		slog.Error("verify", "user", u.ID, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器错误"})
	}
}
