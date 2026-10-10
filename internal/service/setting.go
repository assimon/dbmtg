package service

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
)

// 运行时设置项
const (
	KeyCaptchaMode     = "captcha_mode"
	KeyVerifyTTLDays   = "verify_ttl_days"
	KeyMaxVerifyFail   = "max_verify_fail"
	KeyWelcome         = "welcome"
	KeyWelcomeConfirm  = "welcome_confirm"
	KeyMaxUnreplied    = "max_unreplied"
	KeyReplySignature  = "reply_signature"
	KeyDeliveryReceipt = "delivery_receipt"
	KeyAwayEnabled     = "away_enabled"
	KeyAway            = "away"
	KeyAwayHours       = "away_hours"

	keyBoundGroup = "bound_group_id" // 话题数据所属的管理群，内部使用
)

// CaptchaOff 关闭人机验证
const CaptchaOff = "off"

var (
	// DefaultWelcome 未配置须知，或须知发送失败时使用
	DefaultWelcome = model.StoredMessage{Text: "你好，请直接发送消息，我会尽快回复。"}
	defaultAway    = model.StoredMessage{Text: "我现在不在线，看到消息后会尽快回复。"}
)

type SettingService struct {
	store     *store.Store
	providers map[string]captcha.Provider
	modes     []string // off + 已配置的 provider，按配置顺序
}

func NewSettingService(st *store.Store, providers []captcha.Provider) *SettingService {
	s := &SettingService{store: st, providers: map[string]captcha.Provider{}, modes: []string{CaptchaOff}}
	for _, p := range providers {
		s.providers[p.Name()] = p
		s.modes = append(s.modes, p.Name())
	}
	return s
}

func (s *SettingService) get(key, def string) string {
	if v, ok := s.store.GetSetting(key); ok {
		return v
	}
	return def
}

func (s *SettingService) getInt(key string, def int) int {
	if n, err := strconv.Atoi(s.get(key, "")); err == nil {
		return n
	}
	return def
}

func (s *SettingService) getBool(key string, def bool) bool {
	if v, ok := s.store.GetSetting(key); ok {
		return v == "1"
	}
	return def
}

func (s *SettingService) setBool(key string, on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return s.store.SetSetting(key, v)
}

func (s *SettingService) setInt(key string, n, min int) error {
	if n < min {
		return fmt.Errorf("不能小于 %d", min)
	}
	return s.store.SetSetting(key, strconv.Itoa(n))
}

func (s *SettingService) getMessage(key string, def model.StoredMessage) model.StoredMessage {
	var m model.StoredMessage
	if err := json.Unmarshal([]byte(s.get(key, "")), &m); err != nil || (m.Text == "" && m.MsgID == 0) {
		return def
	}
	return m
}

func (s *SettingService) setMessage(key string, m model.StoredMessage) error {
	if m.Text == "" && m.MsgID == 0 {
		return fmt.Errorf("内容不能为空")
	}
	b, _ := json.Marshal(m)
	return s.store.SetSetting(key, string(b))
}

// CaptchaModes 当前可选的验证方式
func (s *SettingService) CaptchaModes() []string { return s.modes }

// CaptchaMode 当前生效的验证方式；未设置时默认使用第一个已配置的 provider，
// 已设置的 provider 被从配置中移除时回退到 off
func (s *SettingService) CaptchaMode() string {
	def := CaptchaOff
	if len(s.modes) > 1 {
		def = s.modes[1]
	}
	if m := s.get(KeyCaptchaMode, def); slices.Contains(s.modes, m) {
		return m
	}
	return CaptchaOff
}

// CaptchaProvider 当前生效的 provider，关闭验证时返回 nil
func (s *SettingService) CaptchaProvider() captcha.Provider { return s.providers[s.CaptchaMode()] }

func (s *SettingService) SetCaptchaMode(mode string) error {
	if !slices.Contains(s.modes, mode) {
		return fmt.Errorf("不支持的验证方式 %q，可选：%v", mode, s.modes)
	}
	return s.store.SetSetting(KeyCaptchaMode, mode)
}

// VerifyTTLDays 验证有效天数，0 表示永久
func (s *SettingService) VerifyTTLDays() int { return s.getInt(KeyVerifyTTLDays, 0) }

func (s *SettingService) VerifyTTL() time.Duration {
	return time.Duration(s.VerifyTTLDays()) * 24 * time.Hour
}

func (s *SettingService) SetVerifyTTLDays(days int) error { return s.setInt(KeyVerifyTTLDays, days, 0) }

// MaxVerifyFail 连续验证失败多少次自动拉黑
func (s *SettingService) MaxVerifyFail() int { return s.getInt(KeyMaxVerifyFail, 5) }

func (s *SettingService) SetMaxVerifyFail(n int) error { return s.setInt(KeyMaxVerifyFail, n, 1) }

// Welcome 用户 /start 时收到的须知
func (s *SettingService) Welcome() model.StoredMessage {
	return s.getMessage(KeyWelcome, DefaultWelcome)
}

func (s *SettingService) SetWelcome(m model.StoredMessage) error { return s.setMessage(KeyWelcome, m) }

// WelcomeConfirm 是否要求用户点击「我已阅读」后才能发消息
func (s *SettingService) WelcomeConfirm() bool { return s.getBool(KeyWelcomeConfirm, false) }

func (s *SettingService) SetWelcomeConfirm(on bool) error { return s.setBool(KeyWelcomeConfirm, on) }

// MaxUnreplied 管理员回复前用户最多可发送的消息数，0 表示不限制
func (s *SettingService) MaxUnreplied() int { return s.getInt(KeyMaxUnreplied, 0) }

func (s *SettingService) SetMaxUnreplied(n int) error { return s.setInt(KeyMaxUnreplied, n, 0) }

// ReplySignature 回复用户时是否附带回复者名字
func (s *SettingService) ReplySignature() bool { return s.getBool(KeyReplySignature, false) }

func (s *SettingService) SetReplySignature(on bool) error { return s.setBool(KeyReplySignature, on) }

// DeliveryReceipt 消息送达后是否给用户消息加 👌 反应
func (s *SettingService) DeliveryReceipt() bool { return s.getBool(KeyDeliveryReceipt, true) }

func (s *SettingService) SetDeliveryReceipt(on bool) error { return s.setBool(KeyDeliveryReceipt, on) }

// AwayEnabled 离开模式：用户发消息后自动回复
func (s *SettingService) AwayEnabled() bool { return s.getBool(KeyAwayEnabled, false) }

func (s *SettingService) SetAwayEnabled(on bool) error { return s.setBool(KeyAwayEnabled, on) }

func (s *SettingService) Away() model.StoredMessage { return s.getMessage(KeyAway, defaultAway) }

func (s *SettingService) SetAway(m model.StoredMessage) error { return s.setMessage(KeyAway, m) }

// AwayHours 同一用户两次离开回复的最小间隔（小时）
func (s *SettingService) AwayHours() int { return s.getInt(KeyAwayHours, 6) }

func (s *SettingService) SetAwayHours(n int) error { return s.setInt(KeyAwayHours, n, 1) }
