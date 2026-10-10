package bot

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/service"
)

// VerifyPath 验证页路径，由 internal/server 提供
const VerifyPath = "/verify"

// onPrivateMessage 处理私聊消息：超级管理员命令优先，其余按访客处理
func (b *Bot) onPrivateMessage(ctx context.Context, msg *models.Message) {
	if b.cfg.IsAdmin(msg.From.ID) && b.runCommand(ctx, ScopeAdmin, msg, nil) {
		return
	}
	u, err := b.svc.Users.Upsert(ctx, msg.From)
	if err != nil {
		slog.Error("upsert user", "user", msg.From.ID, "err", err)
		return
	}
	if u.Banned || isServiceMessage(msg) {
		return
	}
	if name, _ := parseCommand(msg.Text); name == "start" {
		b.sendWelcome(ctx, u)
		if b.svc.Users.Acked(u) && !b.svc.Users.Verified(u) {
			b.sendVerifyPrompt(ctx, u.ID)
		}
		return
	}

	outcome, err := b.svc.Relay.FromUser(ctx, u, msg)
	if err != nil {
		slog.Error("relay to group", "user", u.ID, "err", err)
		b.tg.Notify(ctx, u.ID, 0, "⚠️ 消息发送失败，请稍后重试。", nil)
		return
	}
	switch outcome {
	case service.NeedAck:
		b.sendWelcome(ctx, u) // 须知本身带「我已阅读」按钮；老用户可能从没收到过须知
	case service.NeedVerify:
		b.sendVerifyPrompt(ctx, u.ID)
	}
}

func (b *Bot) onPrivateEdit(ctx context.Context, msg *models.Message) {
	u, err := b.svc.Users.Get(msg.From.ID)
	if err != nil || u == nil || u.Banned {
		return
	}
	if err := b.svc.Relay.EditFromUser(ctx, u, msg); err != nil {
		slog.Warn("sync user edit", "user", u.ID, "err", err)
	}
}

// sendWelcome 发送须知；开启须知确认且用户未确认时附带「我已阅读」按钮
func (b *Bot) sendWelcome(ctx context.Context, u *model.User) {
	var markup models.ReplyMarkup
	if !b.svc.Users.Acked(u) {
		markup = ackMarkup()
	}
	if err := b.tg.SendStored(ctx, u.ID, b.svc.Settings.Welcome(), markup); err != nil {
		// 须知原消息被删且没有文字时会失败；必须兜底，否则开启须知确认的用户永远拿不到确认按钮
		slog.Warn("send welcome, fallback to default", "user", u.ID, "err", err)
		b.tg.Notify(ctx, u.ID, 0, escape(service.DefaultWelcome.Text), markup)
	}
}

func (b *Bot) sendVerifyPrompt(ctx context.Context, userID int64) {
	b.tg.Notify(ctx, userID, 0, "🛡 请先完成人机验证，验证通过后你的消息会自动送达。", &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "开始验证", WebApp: &models.WebAppInfo{URL: b.cfg.Web.PublicURL + VerifyPath}},
		}},
	})
}

func ackMarkup() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "✅ 我已阅读", CallbackData: cbAck},
	}}}
}
