package bot

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// 回调数据前缀，格式 "<前缀><动作>[:参数]"
const (
	cbSetting = "set:" // 超级管理员 /settings 面板
	cbUser    = "u:"   // 话题资料卡上的用户操作，如 u:ban:123
	cbAck     = "ack"  // 访客确认已阅读须知
)

func (b *Bot) onCallback(ctx context.Context, cq *models.CallbackQuery) {
	var toast string
	switch {
	case strings.HasPrefix(cq.Data, cbSetting):
		toast = b.onSettingCallback(ctx, cq, strings.TrimPrefix(cq.Data, cbSetting))
	case strings.HasPrefix(cq.Data, cbUser):
		toast = b.onUserCallback(ctx, cq, strings.TrimPrefix(cq.Data, cbUser))
	case cq.Data == cbAck:
		toast = b.onAckCallback(ctx, cq)
	}
	b.tg.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: cq.ID, Text: toast})
}

func (b *Bot) onSettingCallback(ctx context.Context, cq *models.CallbackQuery, action string) string {
	if !b.cfg.IsAdmin(cq.From.ID) || cq.Message.Message == nil {
		return "无权限"
	}
	s := b.svc.Settings
	var err error
	switch action {
	case "captcha":
		modes := s.CaptchaModes()
		err = s.SetCaptchaMode(modes[(slices.Index(modes, s.CaptchaMode())+1)%len(modes)])
	case "confirm":
		err = s.SetWelcomeConfirm(!s.WelcomeConfirm())
	case "receipt":
		err = s.SetDeliveryReceipt(!s.DeliveryReceipt())
	case "sig":
		err = s.SetReplySignature(!s.ReplySignature())
	case "away":
		err = s.SetAwayEnabled(!s.AwayEnabled())
	default:
		return ""
	}
	if err != nil {
		return err.Error()
	}
	m := cq.Message.Message
	v := settingsView(s)
	if _, err := b.tg.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID: m.Chat.ID, MessageID: m.ID, Text: v.Text, ParseMode: models.ParseModeHTML, ReplyMarkup: v.Markup,
	}); err != nil {
		slog.Warn("edit settings message", "err", err)
	}
	return "已更新"
}

// onUserCallback 资料卡按钮，管理群成员都可操作
func (b *Bot) onUserCallback(ctx context.Context, cq *models.CallbackQuery, data string) string {
	if cq.Message.Message == nil || cq.Message.Message.Chat.ID != b.cfg.Bot.AdminGroupID {
		return "无权限"
	}
	action, idStr, _ := strings.Cut(data, ":")
	id, _ := strconv.ParseInt(idStr, 10, 64)
	u, err := b.svc.Users.Get(id)
	if err != nil || u == nil {
		return "用户不存在"
	}
	switch action {
	case "ban", "unban":
		if err := b.svc.Users.SetBanned(ctx, u, action == "ban"); err != nil {
			return err.Error()
		}
		if u.Banned {
			return "已拉黑"
		}
		return "已解除拉黑"
	case "release":
		if err := b.svc.Users.ResetUnreplied(u); err != nil {
			return err.Error()
		}
		return "已放行，未回复计数已清零"
	case "refresh":
		b.refreshHeader(ctx, u, true)
		return "已刷新"
	}
	return ""
}

// onAckCallback 访客点击「我已阅读」
func (b *Bot) onAckCallback(ctx context.Context, cq *models.CallbackQuery) string {
	u, err := b.svc.Users.Get(cq.From.ID)
	if err != nil || u == nil || u.Banned {
		return ""
	}
	if u.AckedAt == nil {
		if err := b.svc.Users.MarkAcked(u); err != nil {
			slog.Error("mark acked", "user", u.ID, "err", err)
			return "操作失败，请重试"
		}
	}
	if m := cq.Message.Message; m != nil {
		b.tg.EditMessageReplyMarkup(ctx, &tgbot.EditMessageReplyMarkupParams{ChatID: m.Chat.ID, MessageID: m.ID})
	}
	if !b.svc.Users.Verified(u) {
		b.sendVerifyPrompt(ctx, u.ID)
	} else {
		b.svc.Relay.FlushPending(ctx, u)
	}
	return "感谢阅读"
}
