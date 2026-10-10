package bot

import (
	"context"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/telegram"
)

// onGroupMessage 处理管理群话题内的消息：命令或回复访客
func (b *Bot) onGroupMessage(ctx context.Context, msg *models.Message) {
	if msg.From.ID == b.tg.ID() || !msg.IsTopicMessage || msg.MessageThreadID == 0 {
		return
	}
	u, err := b.svc.Users.ByTopic(msg.MessageThreadID)
	if err != nil || u == nil {
		return
	}
	if b.runCommand(ctx, ScopeTopic, msg, u) {
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
