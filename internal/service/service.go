// Package service 实现业务逻辑：用户、设置、消息中继、人机验证。
// 不依赖 HTTP 框架和 Telegram 更新分发，由 internal/bot 和 internal/server 调用。
package service

import (
	"context"

	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
	"github.com/assimon/dbmtg/internal/telegram"
)

// Events 业务事件回调，由接入层注册（如维护话题资料卡、提示管理员）。
// 未注册的事件直接忽略。
type Events struct {
	// TopicCreated 为用户新建了话题
	TopicCreated func(ctx context.Context, u *model.User)
	// UserChanged 用户资料或状态变化；renamed 表示名字或 username 变了
	UserChanged func(ctx context.Context, u *model.User, renamed bool)
	// LimitReached 用户本轮首次超出未回复上限
	LimitReached func(ctx context.Context, u *model.User, limit int)
}

func (e *Events) topicCreated(ctx context.Context, u *model.User) {
	if e.TopicCreated != nil {
		e.TopicCreated(ctx, u)
	}
}

func (e *Events) userChanged(ctx context.Context, u *model.User, renamed bool) {
	if e.UserChanged != nil {
		e.UserChanged(ctx, u, renamed)
	}
}

func (e *Events) limitReached(ctx context.Context, u *model.User, limit int) {
	if e.LimitReached != nil {
		e.LimitReached(ctx, u, limit)
	}
}

type Services struct {
	Events   *Events
	Settings *SettingService
	Users    *UserService
	Relay    *RelayService
	Verify   *VerifyService
}

// New 组装全部服务
func New(cfg *config.Config, st *store.Store, tg *telegram.Client, providers []captcha.Provider) *Services {
	events := &Events{}
	settings := NewSettingService(st, providers)
	users := &UserService{store: st, settings: settings, events: events}
	relay := &RelayService{
		tg: tg, store: st, users: users, settings: settings, events: events,
		groupID: cfg.Bot.AdminGroupID,
	}
	verify := &VerifyService{tg: tg, users: users, settings: settings, relay: relay}
	return &Services{Events: events, Settings: settings, Users: users, Relay: relay, Verify: verify}
}
