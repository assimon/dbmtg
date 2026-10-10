package bot

import "context"

func topicCommands(b *Bot) []Command {
	users := b.svc.Users
	return []Command{
		{Name: "info", Description: "用户资料", Scope: ScopeTopic,
			Handler: func(_ context.Context, r *Request) Reply {
				return text(UserCard(r.User, users.MessageCount(r.User.ID)))
			}},
		{Name: "ban", Description: "拉黑", Scope: ScopeTopic,
			Handler: func(_ context.Context, r *Request) Reply {
				if err := users.SetBanned(r.User, true); err != nil {
					return errReply(err)
				}
				return text("🚫 已拉黑，对方的消息将不再送达")
			}},
		{Name: "unban", Description: "解除拉黑", Scope: ScopeTopic,
			Handler: func(_ context.Context, r *Request) Reply {
				if err := users.SetBanned(r.User, false); err != nil {
					return errReply(err)
				}
				return text("✅ 已解除拉黑")
			}},
	}
}
