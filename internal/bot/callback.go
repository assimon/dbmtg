package bot

import (
	"context"
	"log/slog"
	"slices"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// 回调数据
const (
	cbCycleCaptcha    = "set:captcha"
	cbToggleSignature = "set:sig"
)

func (b *Bot) onCallback(ctx context.Context, cq *models.CallbackQuery) {
	answer := func(text string) {
		b.tg.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: cq.ID, Text: text})
	}
	if !b.cfg.IsAdmin(cq.From.ID) || cq.Message.Message == nil {
		answer("无权限")
		return
	}

	s := b.svc.Settings
	var err error
	switch cq.Data {
	case cbCycleCaptcha:
		modes := s.CaptchaModes()
		err = s.SetCaptchaMode(modes[(slices.Index(modes, s.CaptchaMode())+1)%len(modes)])
	case cbToggleSignature:
		err = s.SetReplySignature(!s.ReplySignature())
	default:
		answer("")
		return
	}
	if err != nil {
		answer(err.Error())
		return
	}
	answer("已更新")

	m := cq.Message.Message
	v := settingsView(s)
	if _, err := b.tg.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID: m.Chat.ID, MessageID: m.ID, Text: v.Text, ParseMode: models.ParseModeHTML, ReplyMarkup: v.Markup,
	}); err != nil {
		slog.Warn("edit settings message", "err", err)
	}
}
