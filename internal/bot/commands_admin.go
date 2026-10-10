package bot

import (
	"context"
	"strconv"
)

func adminCommands(b *Bot) []Command {
	s := b.svc.Settings
	intArg := func(set func(int) error) func(context.Context, *Request) Reply {
		return func(_ context.Context, r *Request) Reply {
			n, err := strconv.Atoi(r.Args)
			if err != nil {
				return text("请输入整数")
			}
			if err := set(n); err != nil {
				return errReply(err)
			}
			return text("✅ 已保存")
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
		{Name: "setwelcome", Usage: "<内容>", Description: "设置欢迎语", Scope: ScopeAdmin,
			Handler: func(_ context.Context, r *Request) Reply {
				if r.Args == "" {
					return text("用法：/setwelcome 欢迎语内容")
				}
				if err := s.SetWelcomeText(r.Args); err != nil {
					return errReply(err)
				}
				return text("✅ 已保存")
			}},
		{Name: "setttl", Usage: "<天数>", Description: "验证有效期，0 为永久", Scope: ScopeAdmin,
			Handler: intArg(s.SetVerifyTTLDays)},
		{Name: "setmaxfail", Usage: "<次数>", Description: "验证失败多少次自动拉黑", Scope: ScopeAdmin,
			Handler: intArg(s.SetMaxVerifyFail)},
		{Name: "help", Description: "命令帮助", Scope: ScopeAdmin,
			Handler: func(context.Context, *Request) Reply { return text(b.helpText()) }},
	}
}
