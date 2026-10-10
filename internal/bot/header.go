package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/telegram"
)

// 话题资料卡：建话题时发送并置顶，用户资料或状态变化时原地更新。

// onTopicCreated 拉取资料，发送资料卡并置顶
func (b *Bot) onTopicCreated(ctx context.Context, u *model.User) {
	photo := b.fetchProfile(ctx, u)
	text := headerText(u)
	m, err := withProfileButton(u, func(markup models.ReplyMarkup) (*models.Message, error) {
		if photo != "" {
			return telegram.Retry(ctx, func() (*models.Message, error) {
				return b.tg.SendPhoto(ctx, &tgbot.SendPhotoParams{
					ChatID: b.cfg.Bot.AdminGroupID, MessageThreadID: u.TopicID,
					Photo: &models.InputFileString{Data: photo}, Caption: text,
					ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
				})
			})
		}
		return b.tg.SendHTML(ctx, b.cfg.Bot.AdminGroupID, u.TopicID, text, markup)
	})
	if err != nil {
		slog.Warn("send topic header", "user", u.ID, "err", err)
		return
	}
	if err := b.svc.Users.SetHeaderMsgID(u, m.ID); err != nil {
		slog.Warn("save header id", "user", u.ID, "err", err)
	}
	if _, err := b.tg.PinChatMessage(ctx, &tgbot.PinChatMessageParams{
		ChatID: b.cfg.Bot.AdminGroupID, MessageID: m.ID, DisableNotification: true,
	}); err != nil {
		slog.Warn("pin topic header（机器人需要「置顶消息」权限）", "err", err)
	}
}

// onUserChanged 用户资料或状态变化：改名时同步话题标题，并刷新资料卡
func (b *Bot) onUserChanged(ctx context.Context, u *model.User, renamed bool) {
	if renamed {
		if err := b.svc.Relay.RenameTopic(ctx, u); err != nil {
			slog.Warn("rename topic", "user", u.ID, "err", err)
		}
	}
	b.refreshHeader(ctx, u, false)
}

// onLimitReached 用户本轮首次超出未回复上限，分别提示用户和管理员
func (b *Bot) onLimitReached(ctx context.Context, u *model.User, limit int) {
	b.tg.Notify(ctx, u.ID, 0, fmt.Sprintf("⏳ 在收到回复前最多可发送 %d 条消息，请耐心等待。", limit), nil)
	b.tg.Notify(ctx, b.cfg.Bot.AdminGroupID, u.TopicID,
		fmt.Sprintf("⚠️ 该用户已达到未回复上限（%d 条），后续消息已拦截。回复或点击资料卡上的「放行」即可解除。", limit), nil)
}

// refreshHeader 原地更新资料卡文字和按钮；refetch 为 true 时重新拉取 getChat 资料。
// ponytail: 头像只在建话题时获取，编辑无法把纯文字消息换成图片，需要时可删除重发
func (b *Bot) refreshHeader(ctx context.Context, u *model.User, refetch bool) {
	if u.TopicID == 0 || u.HeaderMsgID == 0 {
		return
	}
	if refetch {
		b.fetchProfile(ctx, u)
	}
	text := headerText(u)
	chat := b.cfg.Bot.AdminGroupID
	_, err := withProfileButton(u, func(markup models.ReplyMarkup) (bool, error) {
		_, err := b.tg.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID: chat, MessageID: u.HeaderMsgID, Text: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: tgbot.True()},
		})
		if err != nil && strings.Contains(err.Error(), "no text in the message") {
			// 带头像的资料卡是图片消息，改为编辑说明文字
			_, err = b.tg.EditMessageCaption(ctx, &tgbot.EditMessageCaptionParams{
				ChatID: chat, MessageID: u.HeaderMsgID, Caption: text, ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
			})
		}
		return err == nil, err
	})
	if err != nil && !telegram.IsNotModified(err) {
		slog.Warn("refresh topic header", "user", u.ID, "err", err)
	}
}

// fetchProfile 拉取简介、生日、个人频道并保存，返回最新头像的 file_id（没有则为空）
func (b *Bot) fetchProfile(ctx context.Context, u *model.User) (photoFileID string) {
	if info, err := b.tg.GetChat(ctx, &tgbot.GetChatParams{ChatID: u.ID}); err == nil {
		birthday := ""
		if d := info.Birthdate; d.Day != 0 {
			birthday = fmt.Sprintf("%02d-%02d", d.Month, d.Day)
			if d.Year != 0 {
				birthday = fmt.Sprintf("%d-%s", d.Year, birthday)
			}
		}
		channel := ""
		if c := info.PersonalChat; c != nil {
			channel = c.Title
			if c.Username != "" {
				channel = "@" + c.Username
			}
		}
		if err := b.svc.Users.UpdateProfile(u, info.Bio, birthday, channel); err != nil {
			slog.Warn("save profile", "user", u.ID, "err", err)
		}
	} else {
		slog.Warn("get chat", "user", u.ID, "err", err)
	}
	if ps, err := b.tg.GetUserProfilePhotos(ctx, &tgbot.GetUserProfilePhotosParams{UserID: u.ID, Limit: 1}); err == nil && len(ps.Photos) > 0 {
		sizes := ps.Photos[0]
		photoFileID = sizes[len(sizes)-1].FileID
	}
	return photoFileID
}

// withProfileButton 先带「查看资料」按钮调用 f；用户隐私设置禁止按钮链接时去掉该按钮重试
func withProfileButton[T any](u *model.User, f func(models.ReplyMarkup) (T, error)) (T, error) {
	v, err := f(headerMarkup(u, true))
	if err != nil && strings.Contains(strings.ToUpper(err.Error()), "PRIVACY") {
		return f(headerMarkup(u, false))
	}
	return v, err
}

func headerMarkup(u *model.User, profileButton bool) *models.InlineKeyboardMarkup {
	id := fmt.Sprint(u.ID)
	row1 := []models.InlineKeyboardButton{{Text: "🔄 刷新资料", CallbackData: cbUser + "refresh:" + id}}
	if profileButton {
		row1 = append([]models.InlineKeyboardButton{{Text: "👤 查看资料", URL: "tg://user?id=" + id}}, row1...)
	}
	ban := models.InlineKeyboardButton{Text: "🚫 拉黑", CallbackData: cbUser + "ban:" + id}
	if u.Banned {
		ban = models.InlineKeyboardButton{Text: "✅ 解除拉黑", CallbackData: cbUser + "unban:" + id}
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		row1,
		{ban, {Text: "🔓 放行", CallbackData: cbUser + "release:" + id}},
	}}
}
