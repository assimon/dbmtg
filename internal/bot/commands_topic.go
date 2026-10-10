package bot

import (
	"context"
	"errors"

	"github.com/assimon/dbmtg/internal/service"
)

func topicCommands(b *Bot) []Command {
	users := b.svc.Users
	return []Command{
		{Name: "info", Description: "用户资料", Scope: ScopeTopic,
			Handler: func(_ context.Context, r *Request) Reply {
				return Reply{Text: infoText(r.User, users.MessageCount(r.User.ID)), Markup: headerMarkup(r.User, false)}
			}},
		{Name: "ban", Description: "拉黑", Scope: ScopeTopic,
			Handler: func(ctx context.Context, r *Request) Reply {
				if err := users.SetBanned(ctx, r.User, true); err != nil {
					return errReply(err)
				}
				return text("🚫 已拉黑，对方的消息将不再送达")
			}},
		{Name: "unban", Description: "解除拉黑", Scope: ScopeTopic,
			Handler: func(ctx context.Context, r *Request) Reply {
				if err := users.SetBanned(ctx, r.User, false); err != nil {
					return errReply(err)
				}
				return text("✅ 已解除拉黑")
			}},
		{Name: "del", Description: "回复一条已发出的消息，撤回该消息", Scope: ScopeTopic,
			Handler: func(ctx context.Context, r *Request) Reply {
				rm := r.Msg.ReplyToMessage
				// 话题内未显式回复时，reply_to_message 是话题创建消息
				if rm == nil || rm.ID == r.Msg.MessageThreadID {
					return text("用法：回复要撤回的那条消息，发送 /del")
				}
				err := b.svc.Relay.Retract(ctx, r.User, rm.ID)
				if errors.Is(err, service.ErrNotRetractable) {
					return errReply(err)
				}
				if err != nil {
					return text("❌ 撤回失败（超过 48 小时的消息无法撤回）：" + escape(err.Error()))
				}
				return text("🗑 已撤回")
			}},
	}
}
