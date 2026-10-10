package service

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/store"
)

// 运行时设置项
const (
	KeyCaptchaMode    = "captcha_mode"
	KeyWelcomeText    = "welcome_text"
	KeyVerifyTTLDays  = "verify_ttl_days"
	KeyMaxVerifyFail  = "max_verify_fail"
	KeyReplySignature = "reply_signature"
)

// CaptchaOff 关闭人机验证
const CaptchaOff = "off"

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

func (s *SettingService) WelcomeText() string {
	return s.get(KeyWelcomeText, "你好，请直接发送消息，我会尽快回复。")
}

func (s *SettingService) SetWelcomeText(text string) error {
	return s.store.SetSetting(KeyWelcomeText, text)
}

// VerifyTTLDays 验证有效天数，0 表示永久
func (s *SettingService) VerifyTTLDays() int { return s.getInt(KeyVerifyTTLDays, 0) }

func (s *SettingService) VerifyTTL() time.Duration {
	return time.Duration(s.VerifyTTLDays()) * 24 * time.Hour
}

func (s *SettingService) SetVerifyTTLDays(days int) error {
	if days < 0 {
		return fmt.Errorf("天数不能为负")
	}
	return s.store.SetSetting(KeyVerifyTTLDays, strconv.Itoa(days))
}

// MaxVerifyFail 连续验证失败多少次自动拉黑
func (s *SettingService) MaxVerifyFail() int { return s.getInt(KeyMaxVerifyFail, 5) }

func (s *SettingService) SetMaxVerifyFail(n int) error {
	if n < 1 {
		return fmt.Errorf("次数至少为 1")
	}
	return s.store.SetSetting(KeyMaxVerifyFail, strconv.Itoa(n))
}

// ReplySignature 回复用户时是否附带回复者名字
func (s *SettingService) ReplySignature() bool { return s.get(KeyReplySignature, "0") == "1" }

func (s *SettingService) SetReplySignature(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return s.store.SetSetting(KeyReplySignature, v)
}
