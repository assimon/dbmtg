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

// headerText 话题资料卡内容，分区排版：
//
//	名字（粗体，点击打开资料页）
//	ID · @username · 语言 · Premium
//	┃ 简介（可折叠引用）
//	📢 个人频道 · 🎂 生日
//	状态 · 首次来访时间
func headerText(u *model.User) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<a href=\"tg://user?id=%d\"><b>%s</b></a>\n",
		u.ID, escape(service.DisplayName(u.FirstName, u.LastName)))

	meta := []string{fmt.Sprintf("<code>%d</code>", u.ID)}
	if u.Username != "" {
		meta = append(meta, "@"+u.Username)
	}
	if u.LanguageCode != "" {
		meta = append(meta, escape(languageName(u.LanguageCode)))
	}
	if u.IsPremium {
		meta = append(meta, "Premium")
	}
	b.WriteString(strings.Join(meta, " · "))

	if u.Bio != "" {
		fmt.Fprintf(&b, "\n<blockquote expandable>%s</blockquote>", escape(u.Bio))
	}

	var extra []string
	if u.PersonalChannel != "" {
		extra = append(extra, "📢 "+escape(u.PersonalChannel))
	}
	if u.Birthday != "" {
		extra = append(extra, "🎂 "+escape(u.Birthday))
	}
	if len(extra) > 0 {
		b.WriteString("\n" + strings.Join(extra, " · "))
	}

	status := "⏳ 未验证"
	switch {
	case u.Banned:
		status = "🚫 已拉黑"
	case u.VerifiedAt != nil:
		status = "✅ 已验证"
	}
	fmt.Fprintf(&b, "\n\n%s · <i>首次来访 %s</i>", status, u.CreatedAt.Format(timeLayout))
	return b.String()
}

// infoText /info 输出：资料卡 + 统计
func infoText(u *model.User, msgCount int64) string {
	return fmt.Sprintf("%s\n💬 %d 条消息 · 📮 未回复 %d · ❌ 验证失败 %d",
		headerText(u), msgCount, u.Unreplied, u.FailCount)
}

var languageNames = map[string]string{
	"zh-hans": "简体中文", "zh-hant": "繁體中文", "zh": "中文", "en": "English", "ru": "Русский",
	"ja": "日本語", "ko": "한국어", "es": "Español", "fr": "Français", "de": "Deutsch",
	"pt-br": "Português", "ar": "العربية", "fa": "فارسی", "tr": "Türkçe", "uk": "Українська",
	"vi": "Tiếng Việt", "th": "ไทย", "id": "Bahasa Indonesia",
}

// languageName 常见语言代码显示为语言名，其它原样返回
func languageName(code string) string {
	if n, ok := languageNames[strings.ToLower(code)]; ok {
		return n
	}
	return code
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
