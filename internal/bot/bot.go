// Package bot 是 Telegram 接入层：接收更新、分发到命令和业务服务。
package bot

import (
	"context"
	"fmt"
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
	svc.Events.TopicCreated = b.onTopicCreated
	svc.Events.UserChanged = b.onUserChanged
	svc.Events.LimitReached = b.onLimitReached
	tg.RegisterHandlerMatchFunc(func(*models.Update) bool { return true }, b.dispatch)
	return b
}

// WebhookHandler webhook 模式下挂到 HTTP 服务上
func (b *Bot) WebhookHandler() http.Handler { return b.tg.WebhookHandler() }

// Run 开始接收更新，阻塞直到 ctx 结束
func (b *Bot) Run(ctx context.Context) error {
	if !b.cfg.SetupMode() {
		b.syncCommands(ctx)
	}
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
	if b.cfg.SetupMode() {
		b.onSetup(ctx, upd)
		return
	}
	if m := upd.Message; m != nil && m.From != nil && !b.isGroup(m) && !b.cfg.IsAdmin(m.From.ID) &&
		(m.Chat.Type == models.ChatTypeGroup || m.Chat.Type == models.ChatTypeSupergroup) {
		// 被陌生人拉进了管理群以外的群，直接退出（超级管理员拉的群保留，方便更换管理群）
		slog.Info("leave unknown group", "chat", m.Chat.ID, "title", m.Chat.Title)
		b.tg.LeaveChat(ctx, &tgbot.LeaveChatParams{ChatID: m.Chat.ID})
		return
	}
	switch {
	case upd.CallbackQuery != nil:
		b.onCallback(ctx, upd.CallbackQuery)
	case upd.Message != nil && upd.Message.From != nil:
		if b.isGroup(upd.Message) {
			b.onGroupMessage(ctx, upd.Message)
		} else if upd.Message.Chat.Type == models.ChatTypePrivate {
			b.onPrivateMessage(ctx, upd.Message)
		}
	case upd.EditedMessage != nil && upd.EditedMessage.From != nil:
		if b.isGroup(upd.EditedMessage) {
			b.onGroupEdit(ctx, upd.EditedMessage)
		} else if upd.EditedMessage.Chat.Type == models.ChatTypePrivate {
			b.onPrivateEdit(ctx, upd.EditedMessage)
		}
	}
}

func (b *Bot) isGroup(msg *models.Message) bool { return msg.Chat.ID == b.cfg.Bot.AdminGroupID }

// onSetup 初始化模式：对任何消息回复当前会话 ID 和发送者 ID
func (b *Bot) onSetup(ctx context.Context, upd *models.Update) {
	m := upd.Message
	if m == nil || m.From == nil {
		return
	}
	b.tg.Notify(ctx, m.Chat.ID, m.MessageThreadID, fmt.Sprintf(`🔧 <b>初始化模式</b>
当前会话 ID：<code>%d</code>
你的用户 ID：<code>%d</code>

在管理群里发送 /id 得到群 ID，填入 <code>bot.admin_group_id</code>；
你的用户 ID 填入 <code>bot.admin_ids</code>，然后重启机器人。`, m.Chat.ID, m.From.ID), nil)
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
