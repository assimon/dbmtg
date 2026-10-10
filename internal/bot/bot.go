// Package bot 是 Telegram 接入层：接收更新、分发到命令和业务服务。
package bot

import (
	"context"
	"log/slog"
	"net/http"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/service"
	"github.com/assimon/dbmtg/internal/telegram"
)

type Bot struct {
	cfg      *config.Config
	tg       *telegram.Client
	svc      *service.Services
	commands []Command
}

func New(cfg *config.Config, tg *telegram.Client, svc *service.Services) *Bot {
	b := &Bot{cfg: cfg, tg: tg, svc: svc}
	b.commands = append(adminCommands(b), topicCommands(b)...)
	tg.RegisterHandlerMatchFunc(func(*models.Update) bool { return true }, b.dispatch)
	return b
}

// WebhookHandler webhook 模式下挂到 HTTP 服务上
func (b *Bot) WebhookHandler() http.Handler { return b.tg.WebhookHandler() }

// Run 开始接收更新，阻塞直到 ctx 结束
func (b *Bot) Run(ctx context.Context) error {
	b.syncCommands(ctx)
	if b.cfg.Bot.Mode == config.ModeWebhook {
		if _, err := b.tg.SetWebhook(ctx, &tgbot.SetWebhookParams{
			URL:            b.cfg.Web.PublicURL + WebhookPath,
			SecretToken:    b.tg.WebhookSecret,
			AllowedUpdates: telegram.AllowedUpdates,
		}); err != nil {
			return err
		}
		b.tg.StartWebhook(ctx)
		return nil
	}
	if _, err := b.tg.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{}); err != nil {
		return err
	}
	b.tg.Start(ctx)
	return nil
}

// WebhookPath webhook 回调路径
const WebhookPath = "/tg/webhook"

func (b *Bot) dispatch(ctx context.Context, _ *tgbot.Bot, upd *models.Update) {
	switch {
	case upd.CallbackQuery != nil:
		b.onCallback(ctx, upd.CallbackQuery)
	case upd.Message == nil || upd.Message.From == nil:
	case upd.Message.Chat.ID == b.cfg.Bot.AdminGroupID:
		b.onGroupMessage(ctx, upd.Message)
	case upd.Message.Chat.Type == models.ChatTypePrivate:
		b.onPrivateMessage(ctx, upd.Message)
	}
}

// syncCommands 把命令注册到 Telegram 的命令菜单
func (b *Bot) syncCommands(ctx context.Context) {
	set := func(scope models.BotCommandScope, s Scope) {
		if _, err := b.tg.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{Commands: b.menu(s), Scope: scope}); err != nil {
			slog.Warn("set commands", "scope", s, "err", err)
		}
	}
	set(&models.BotCommandScopeAllPrivateChats{}, ScopeUser)
	set(&models.BotCommandScopeChat{ChatID: b.cfg.Bot.AdminGroupID}, ScopeTopic)
	for _, id := range b.cfg.Bot.AdminIDs {
		set(&models.BotCommandScopeChat{ChatID: id}, ScopeAdmin)
	}
}
