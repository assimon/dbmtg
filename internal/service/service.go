// Package service 实现业务逻辑：用户、设置、消息中继、人机验证。
// 不依赖 HTTP 框架和 Telegram 更新分发，由 internal/bot 和 internal/server 调用。
package service

import (
	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
	"github.com/assimon/dbmtg/internal/telegram"
)

type Services struct {
	Settings *SettingService
	Users    *UserService
	Relay    *RelayService
	Verify   *VerifyService
}

// New 组装全部服务。topicHeader 用于渲染新话题的首条用户资料消息。
func New(cfg *config.Config, st *store.Store, tg *telegram.Client, providers []captcha.Provider,
	topicHeader func(*model.User) string) *Services {
	settings := NewSettingService(st, providers)
	users := &UserService{store: st, settings: settings}
	relay := &RelayService{
		tg: tg, store: st, users: users, settings: settings,
		groupID: cfg.Bot.AdminGroupID, topicHeader: topicHeader,
	}
	verify := &VerifyService{tg: tg, users: users, settings: settings, relay: relay}
	return &Services{Settings: settings, Users: users, Relay: relay, Verify: verify}
}
