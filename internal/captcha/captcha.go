// Package captcha 定义人机验证提供方。
//
// 新增一种验证方式：实现 Provider，并在 FromConfig 中按配置注册即可，
// 验证页和 /settings 会自动识别。
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/assimon/dbmtg/internal/config"
)

type Provider interface {
	// Name 唯一标识，同时作为 settings 中 captcha_mode 的取值
	Name() string
	// Widget 返回嵌入验证页的 HTML（含所需脚本）。
	// 用户完成验证后，组件需调用页面全局函数 onCaptchaToken(token)。
	Widget() template.HTML
	// Verify 服务端校验前端拿到的 token
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// FromConfig 返回所有已完整配置的 Provider
func FromConfig(c config.Captcha) []Provider {
	var ps []Provider
	if c.Turnstile.SiteKey != "" && c.Turnstile.Secret != "" {
		ps = append(ps, &Turnstile{SiteKey: c.Turnstile.SiteKey, Secret: c.Turnstile.Secret})
	}
	if c.Cap.URL != "" && c.Cap.SiteKey != "" && c.Cap.Secret != "" {
		ps = append(ps, &Cap{URL: c.Cap.URL, SiteKey: c.Cap.SiteKey, Secret: c.Cap.Secret})
	}
	return ps
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// siteverify 发送校验请求并解析通用的 {"success": bool} 响应
func siteverify(req *http.Request) (bool, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var res struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false, fmt.Errorf("decode siteverify response (HTTP %d): %w", resp.StatusCode, err)
	}
	return res.Success, nil
}
