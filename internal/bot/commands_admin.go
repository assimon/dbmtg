package bot

import (
	"context"
	"strconv"

	"github.com/assimon/dbmtg/internal/model"
)

func adminCommands(b *Bot) []Command {
	s := b.svc.Settings
	saved := text("✅ 已保存")
	intArg := func(set func(int) error) func(context.Context, *Request) Reply {
		return func(_ context.Context, r *Request) Reply {
			n, err := strconv.Atoi(r.Args)
			if err != nil {
				return text("请输入整数")
			}
			if err := set(n); err != nil {
				return errReply(err)
			}
			return saved
		}
	}
	// storedArg 支持两种写法：回复一条消息发送命令（保留格式和媒体），或命令后直接跟文字
	storedArg := func(usage string, set func(model.StoredMessage) error) func(context.Context, *Request) Reply {
		return func(ctx context.Context, r *Request) Reply {
			sm, ok := storedFromRequest(r)
			if !ok {
				return text(usage)
			}
			if err := set(sm); err != nil {
				return errReply(err)
			}
			b.tg.SendStored(ctx, r.Msg.Chat.ID, sm, nil)
			if sm.MsgID != 0 {
				return text("✅ 已保存，以上为预览。请勿删除被引用的原消息，否则会退回为纯文字。")
			}
			return text("✅ 已保存，以上为预览。")
		}
	}

	return []Command{
		{Name: "settings", Description: "查看和修改设置", Scope: ScopeAdmin,
			Handler: func(context.Context, *Request) Reply { return settingsView(s) }},
		{Name: "stats", Description: "统计", Scope: ScopeAdmin,
			Handler: func(context.Context, *Request) Reply {
				st, err := b.svc.Users.Stats()
				if err != nil {
					return errReply(err)
				}
				return text(statsText(st))
			}},
		{Name: "welcome", Description: "预览须知", Scope: ScopeAdmin,
			Handler: func(ctx context.Context, r *Request) Reply {
				b.tg.SendStored(ctx, r.Msg.Chat.ID, s.Welcome(), nil)
				return Reply{}
			}},
		{Name: "setwelcome", Usage: "<内容> 或回复一条消息", Description: "设置须知", Scope: ScopeAdmin,
			Handler: storedArg("用法：回复一条消息发送 /setwelcome（保留格式、图片），或 /setwelcome 文字内容", s.SetWelcome)},
		{Name: "setmaxunreplied", Usage: "<条数>", Description: "管理员回复前用户最多可发几条，0 为不限", Scope: ScopeAdmin,
			Handler: intArg(s.SetMaxUnreplied)},
		{Name: "setaway", Usage: "<内容> 或回复一条消息", Description: "设置离开模式自动回复", Scope: ScopeAdmin,
			Handler: storedArg("用法：回复一条消息发送 /setaway，或 /setaway 文字内容", s.SetAway)},
		{Name: "setawayhours", Usage: "<小时>", Description: "离开回复对同一用户的间隔", Scope: ScopeAdmin,
			Handler: intArg(s.SetAwayHours)},
		{Name: "setttl", Usage: "<天数>", Description: "验证有效期，0 为永久", Scope: ScopeAdmin,
			Handler: intArg(s.SetVerifyTTLDays)},
		{Name: "setmaxfail", Usage: "<次数>", Description: "验证失败多少次自动拉黑", Scope: ScopeAdmin,
			Handler: intArg(s.SetMaxVerifyFail)},
		{Name: "help", Description: "命令帮助", Scope: ScopeAdmin,
			Handler: func(context.Context, *Request) Reply { return text(b.helpText()) }},
	}
}

func storedFromRequest(r *Request) (model.StoredMessage, bool) {
	if rm := r.Msg.ReplyToMessage; rm != nil {
		t, es := rm.Text, rm.Entities
		if t == "" {
			t, es = rm.Caption, rm.CaptionEntities
		}
		return model.StoredMessage{Text: t, Entities: es, ChatID: rm.Chat.ID, MsgID: rm.ID}, true
	}
	if r.Args != "" {
		return model.StoredMessage{Text: r.Args, Entities: r.ArgsEntities}, true
	}
	return model.StoredMessage{}, false
}
