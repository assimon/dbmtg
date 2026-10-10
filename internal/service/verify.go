package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/assimon/dbmtg/internal/telegram"
)

var (
	ErrCaptchaDisabled = errors.New("当前无需验证")
	ErrUserNotFound    = errors.New("请先私聊机器人")
	ErrBanned          = errors.New("你已被拉黑")
	ErrVerifyFailed    = errors.New("验证失败，请重试")
	ErrCaptchaService  = errors.New("验证服务暂时不可用，请稍后再试")
)

type VerifyService struct {
	tg       *telegram.Client
	users    *UserService
	settings *SettingService
	relay    *RelayService
}

// Verify 校验 captcha token；失败累计次数，达到上限自动拉黑；成功后投递暂存消息
func (s *VerifyService) Verify(ctx context.Context, userID int64, token, remoteIP string) error {
	p := s.settings.CaptchaProvider()
	if p == nil {
		return ErrCaptchaDisabled
	}
	u, err := s.users.Get(userID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrUserNotFound
	}
	if u.Banned {
		return ErrBanned
	}

	ok, err := p.Verify(ctx, token, remoteIP)
	if err != nil {
		return fmt.Errorf("%w: %s siteverify: %v", ErrCaptchaService, p.Name(), err)
	}
	if !ok {
		if err := s.users.RecordVerifyFail(u); err != nil {
			return err
		}
		if u.Banned {
			return ErrBanned
		}
		return ErrVerifyFailed
	}

	if err := s.users.MarkVerified(u); err != nil {
		return err
	}
	// 用独立 context，避免 HTTP 请求结束导致投递中断
	bg := context.WithoutCancel(ctx)
	s.tg.Notify(bg, u.ID, 0, "✅ 验证通过，现在可以直接发送消息了。", nil)
	go s.relay.FlushPending(bg, u)
	return nil
}
