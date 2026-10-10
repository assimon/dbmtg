package bot

import (
	"fmt"
	"html"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/service"
)

func escape(s string) string { return html.EscapeString(s) }

// TopicHeader 新话题的首条消息
func TopicHeader(u *model.User) string { return UserCard(u, 0) }

// UserCard 访客资料卡
func UserCard(u *model.User, msgCount int64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "👤 <a href=\"tg://user?id=%d\">%s</a>\n", u.ID, escape(service.DisplayName(u.FirstName, u.LastName)))
	if u.Username != "" {
		fmt.Fprintf(&b, "@%s\n", u.Username)
	}
	fmt.Fprintf(&b, "ID: <code>%d</code>\n首次来访: %s\n", u.ID, u.CreatedAt.Format("2006-01-02 15:04"))
	if u.VerifiedAt != nil {
		fmt.Fprintf(&b, "验证时间: %s\n", u.VerifiedAt.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(&b, "消息数: %d", msgCount)
	if u.Banned {
		b.WriteString("\n🚫 已拉黑")
	}
	return b.String()
}

func onOff(on bool) string {
	if on {
		return "开"
	}
	return "关"
}

func settingsView(s *service.SettingService) Reply {
	ttl := "永久"
	if n := s.VerifyTTLDays(); n > 0 {
		ttl = fmt.Sprintf("%d 天", n)
	}
	t := fmt.Sprintf(`⚙️ <b>当前设置</b>
验证方式：<b>%s</b>（可选：%s）
验证有效期：%s
失败拉黑次数：%d
回复署名：%s
欢迎语：%s

发送 /help 查看修改命令`,
		s.CaptchaMode(), strings.Join(s.CaptchaModes(), " / "), ttl, s.MaxVerifyFail(),
		onOff(s.ReplySignature()), escape(s.WelcomeText()))
	return Reply{Text: t, Markup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "切换验证方式", CallbackData: cbCycleCaptcha}},
		{{Text: "切换回复署名", CallbackData: cbToggleSignature}},
	}}}
}

func statsText(st service.Stats) string {
	return fmt.Sprintf("📊 <b>统计</b>\n用户：%d\n已验证：%d\n已拉黑：%d\n消息：%d",
		st.Total, st.Verified, st.Banned, st.Messages)
}
