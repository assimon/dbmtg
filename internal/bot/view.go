package bot

import (
	"fmt"
	"html"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/service"
)

const timeLayout = "2006-01-02 15:04"

func escape(s string) string { return html.EscapeString(s) }

// headerText 话题资料卡内容
func headerText(u *model.User) string {
	var b strings.Builder
	name := escape(service.DisplayName(u.FirstName, u.LastName))
	fmt.Fprintf(&b, "👤 <a href=\"tg://user?id=%d\">%s</a>", u.ID, name)
	if u.IsPremium {
		b.WriteString(" ⭐")
	}
	b.WriteString("\n")
	if u.Username != "" {
		fmt.Fprintf(&b, "@%s · ", u.Username)
	}
	fmt.Fprintf(&b, "ID <code>%d</code>\n", u.ID)
	line := func(icon, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%s %s\n", icon, escape(v))
		}
	}
	line("🌐", u.LanguageCode)
	line("📝", u.Bio)
	line("🎂", u.Birthday)
	line("📢", u.PersonalChannel)
	fmt.Fprintf(&b, "🕐 首次来访 %s\n", u.CreatedAt.Format(timeLayout))
	if u.VerifiedAt != nil {
		fmt.Fprintf(&b, "✅ 已验证 %s", u.VerifiedAt.Format(timeLayout))
	} else {
		b.WriteString("⏳ 未验证")
	}
	if u.Banned {
		b.WriteString("\n🚫 已拉黑")
	}
	return b.String()
}

// infoText /info 输出：资料卡 + 统计
func infoText(u *model.User, msgCount int64) string {
	return fmt.Sprintf("%s\n\n💬 消息数 %d\n📮 未回复 %d\n❌ 验证失败 %d 次",
		headerText(u), msgCount, u.Unreplied, u.FailCount)
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
	unreplied := "不限"
	if n := s.MaxUnreplied(); n > 0 {
		unreplied = fmt.Sprintf("%d 条", n)
	}
	t := fmt.Sprintf(`⚙️ <b>当前设置</b>

🛡 验证方式：<b>%s</b>（可选：%s）
⏱ 验证有效期：%s
❌ 失败拉黑次数：%d
📜 须知确认：%s
📮 未回复上限：%s
👌 送达回执：%s
✍️ 回复署名：%s
🌙 离开模式：%s（同一用户间隔 %d 小时）

/welcome 预览须知 · /help 查看全部命令`,
		s.CaptchaMode(), strings.Join(s.CaptchaModes(), " / "), ttl, s.MaxVerifyFail(),
		onOff(s.WelcomeConfirm()), unreplied, onOff(s.DeliveryReceipt()), onOff(s.ReplySignature()),
		onOff(s.AwayEnabled()), s.AwayHours())

	toggle := func(label string, on bool, action string) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: label + "：" + onOff(on), CallbackData: cbSetting + action}
	}
	return Reply{Text: t, Markup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "🛡 切换验证方式", CallbackData: cbSetting + "captcha"}},
		{toggle("📜 须知确认", s.WelcomeConfirm(), "confirm"), toggle("👌 送达回执", s.DeliveryReceipt(), "receipt")},
		{toggle("✍️ 回复署名", s.ReplySignature(), "sig"), toggle("🌙 离开模式", s.AwayEnabled(), "away")},
	}}}
}

func statsText(st service.Stats) string {
	return fmt.Sprintf("📊 <b>统计</b>\n用户：%d\n已验证：%d\n已拉黑：%d\n消息：%d",
		st.Total, st.Verified, st.Banned, st.Messages)
}
