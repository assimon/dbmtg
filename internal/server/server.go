// Package server 是 HTTP 接入层（gin）：验证页、验证接口、webhook。
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/service"
)

const apiVerifyPath = "/api/verify"

type Server struct {
	cfg *config.Config
	svc *service.Services
}

// New 创建 HTTP 服务。webhook 非 nil 时挂载到 webhookPath。
func New(cfg *config.Config, svc *service.Services, verifyPath, webhookPath string, webhook http.Handler) *http.Server {
	s := &Server{cfg: cfg, svc: svc}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET(verifyPath, s.verifyPage)
	r.POST(apiVerifyPath, s.verifyAPI)
	if webhook != nil {
		r.POST(webhookPath, gin.WrapH(webhook))
	}
	return &http.Server{Addr: cfg.Web.Listen, Handler: r}
}
