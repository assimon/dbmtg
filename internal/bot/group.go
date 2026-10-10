package bot

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/telegram"
)

// topicUser 返回话题消息对应的访客；非话题消息、机器人自己的消息返回 nil
func (b *Bot) topicUser(msg *models.Message) *model.User {
	if msg.From.ID == b.tg.ID() || !msg.IsTopicMessage || msg.MessageThreadID == 0 {
		return nil
	}
	u, err := b.svc.Users.ByTopic(msg.MessageThreadID)
	if err != nil {
		slog.Error("user by topic", "topic", msg.MessageThreadID, "err", err)
	}
	return u
}

// onGroupMessage 处理管理群话题内的消息：命令或回复访客
func (b *Bot) onGroupMessage(ctx context.Context, msg *models.Message) {
	u := b.topicUser(msg)
	if u == nil || b.runCommand(ctx, ScopeTopic, msg, u) {
		return
	}
	if err := b.svc.Relay.ToUser(ctx, u, msg); err != nil {
		hint := escape(err.Error())
		if telegram.IsForbidden(err) {
			hint = "对方已屏蔽机器人或已注销"
		}
		b.tg.Notify(ctx, msg.Chat.ID, msg.MessageThreadID, "⚠️ 发送失败："+hint, nil)
	}
}

func (b *Bot) onGroupEdit(ctx context.Context, msg *models.Message) {
	u := b.topicUser(msg)
	if u == nil {
		return
	}
	if err := b.svc.Relay.EditFromAdmin(ctx, u, msg); err != nil {
		b.tg.Notify(ctx, msg.Chat.ID, msg.MessageThreadID, "⚠️ 编辑同步失败："+escape(err.Error()), nil)
	}
}
