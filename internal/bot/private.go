package bot

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot/models"
)

// onPrivateMessage 处理私聊消息：超级管理员命令优先，其余按访客处理
func (b *Bot) onPrivateMessage(ctx context.Context, msg *models.Message) {
	if b.cfg.IsAdmin(msg.From.ID) && b.runCommand(ctx, ScopeAdmin, msg, nil) {
		return
	}
	f := msg.From
	u, err := b.svc.Users.Upsert(f.ID, f.FirstName, f.LastName, f.Username)
	if err != nil {
		slog.Error("upsert user", "user", f.ID, "err", err)
		return
	}
	if u.Banned {
		return
	}
	if name, _ := parseCommand(msg.Text); name == "start" {
		b.tg.Notify(ctx, u.ID, 0, escape(b.svc.Settings.WelcomeText()), nil)
		if !b.svc.Users.Verified(u) {
			b.sendVerifyPrompt(ctx, u.ID)
		}
		return
	}
	delivered, err := b.svc.Relay.FromUser(ctx, u, msg)
	if err != nil {
		slog.Error("relay to group", "user", u.ID, "err", err)
		return
	}
	if !delivered {
		b.sendVerifyPrompt(ctx, u.ID)
	}
}

func (b *Bot) sendVerifyPrompt(ctx context.Context, userID int64) {
	b.tg.Notify(ctx, userID, 0, "🛡 请先完成人机验证，验证通过后你的消息会自动送达。", &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: "开始验证", WebApp: &models.WebAppInfo{URL: b.cfg.Web.PublicURL + VerifyPath}},
		}},
	})
}

// VerifyPath 验证页路径，由 internal/server 提供
const VerifyPath = "/verify"
