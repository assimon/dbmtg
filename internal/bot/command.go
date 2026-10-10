package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
)

// Scope 命令生效的位置
type Scope int

const (
	ScopeUser  Scope = iota // 访客私聊（仅用于菜单展示，如 /start）
	ScopeAdmin              // 超级管理员私聊
	ScopeTopic              // 管理群话题内
)

// Request 命令调用上下文
type Request struct {
	Msg  *models.Message
	Args string
	User *model.User // ScopeTopic 时为话题对应的访客
}

// Reply 命令的回复，Text 为空则不回复
type Reply struct {
	Text   string // HTML
	Markup models.ReplyMarkup
}

// Command 新增命令：在 adminCommands / topicCommands 中追加一项即可，
// 帮助文本和 Telegram 命令菜单会自动更新。
type Command struct {
	Name        string
	Usage       string // 参数说明，如 "<天数>"
	Description string
	Scope       Scope
	Handler     func(ctx context.Context, r *Request) Reply
}

func text(s string) Reply { return Reply{Text: s} }

func errReply(err error) Reply { return text("❌ " + err.Error()) }

// parseCommand 解析 "/cmd@bot args"
func parseCommand(s string) (name, args string) {
	if !strings.HasPrefix(s, "/") {
		return "", ""
	}
	first, rest, _ := strings.Cut(s, " ")
	name, _, _ = strings.Cut(first[1:], "@")
	return strings.ToLower(name), strings.TrimSpace(rest)
}

// runCommand 执行命令，返回 false 表示不是该范围内的命令
func (b *Bot) runCommand(ctx context.Context, scope Scope, msg *models.Message, u *model.User) bool {
	name, args := parseCommand(msg.Text)
	if name == "" {
		return false
	}
	for _, c := range b.commands {
		if c.Scope == scope && c.Name == name {
			r := c.Handler(ctx, &Request{Msg: msg, Args: args, User: u})
			if r.Text != "" {
				b.tg.Notify(ctx, msg.Chat.ID, msg.MessageThreadID, r.Text, r.Markup)
			}
			return true
		}
	}
	return false
}

func (b *Bot) menu(scope Scope) []models.BotCommand {
	if scope == ScopeUser {
		return []models.BotCommand{{Command: "start", Description: "开始"}}
	}
	var cs []models.BotCommand
	for _, c := range b.commands {
		if c.Scope == scope {
			cs = append(cs, models.BotCommand{Command: c.Name, Description: c.Description})
		}
	}
	return cs
}

func (b *Bot) helpText() string {
	var sb strings.Builder
	for _, s := range []struct {
		scope Scope
		title string
	}{{ScopeAdmin, "私聊机器人（超级管理员）"}, {ScopeTopic, "管理群话题内"}} {
		fmt.Fprintf(&sb, "<b>%s</b>\n", s.title)
		for _, c := range b.commands {
			if c.Scope == s.scope {
				usage := ""
				if c.Usage != "" {
					usage = " " + escape(c.Usage)
				}
				fmt.Fprintf(&sb, "/%s%s — %s\n", c.Name, usage, c.Description)
			}
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
