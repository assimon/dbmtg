package service

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
)

type UserService struct {
	store    *store.Store
	settings *SettingService
	events   *Events
}

// Upsert 创建或更新用户的基础资料，名字或 username 变化时触发 UserChanged
func (s *UserService) Upsert(ctx context.Context, f *models.User) (*model.User, error) {
	u, err := s.store.GetUser(f.ID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u = &model.User{ID: f.ID, FirstName: f.FirstName, LastName: f.LastName, Username: f.Username,
			LanguageCode: f.LanguageCode, IsPremium: f.IsPremium}
		return u, s.store.CreateUser(u)
	}
	renamed := u.FirstName != f.FirstName || u.LastName != f.LastName || u.Username != f.Username
	if !renamed && u.LanguageCode == f.LanguageCode && u.IsPremium == f.IsPremium {
		return u, nil
	}
	// 只更新资料字段，避免覆盖其它 goroutine 同时写入的话题、计数等状态
	u.FirstName, u.LastName, u.Username = f.FirstName, f.LastName, f.Username
	u.LanguageCode, u.IsPremium = f.LanguageCode, f.IsPremium
	if err := s.store.UpdateUser(u.ID, map[string]any{
		"first_name": f.FirstName, "last_name": f.LastName, "username": f.Username,
		"language_code": f.LanguageCode, "is_premium": f.IsPremium,
	}); err != nil {
		return nil, err
	}
	if renamed {
		s.events.userChanged(ctx, u, true)
	}
	return u, nil
}

// UpdateProfile 保存从 getChat 拉取的扩展资料
func (s *UserService) UpdateProfile(u *model.User, bio, birthday, channel string) error {
	u.Bio, u.Birthday, u.PersonalChannel = bio, birthday, channel
	return s.store.UpdateUser(u.ID, map[string]any{"bio": bio, "birthday": birthday, "personal_channel": channel})
}

func (s *UserService) SetHeaderMsgID(u *model.User, id int) error {
	u.HeaderMsgID = id
	return s.store.UpdateUser(u.ID, map[string]any{"header_msg_id": id})
}

func (s *UserService) Get(id int64) (*model.User, error) { return s.store.GetUser(id) }

func (s *UserService) ByTopic(topicID int) (*model.User, error) { return s.store.UserByTopic(topicID) }

// Acked 用户是否已确认须知（未开启确认时视为已确认）
func (s *UserService) Acked(u *model.User) bool {
	return !s.settings.WelcomeConfirm() || u.AckedAt != nil
}

func (s *UserService) MarkAcked(u *model.User) error {
	now := time.Now()
	u.AckedAt = &now
	return s.store.UpdateUser(u.ID, map[string]any{"acked_at": now})
}

// Verified 用户当前是否无需（再）验证
func (s *UserService) Verified(u *model.User) bool {
	if s.settings.CaptchaMode() == CaptchaOff {
		return true
	}
	if u.VerifiedAt == nil {
		return false
	}
	ttl := s.settings.VerifyTTL()
	return ttl <= 0 || time.Since(*u.VerifiedAt) < ttl
}

// MarkVerified 标记验证通过并清零失败次数
func (s *UserService) MarkVerified(ctx context.Context, u *model.User) error {
	now := time.Now()
	if err := s.store.UpdateUser(u.ID, map[string]any{"verified_at": now, "fail_count": 0}); err != nil {
		return err
	}
	u.VerifiedAt, u.FailCount = &now, 0
	s.events.userChanged(ctx, u, false)
	return nil
}

// RecordVerifyFail 累计一次验证失败，达到上限自动拉黑
func (s *UserService) RecordVerifyFail(ctx context.Context, u *model.User) error {
	u.FailCount++
	wasBanned := u.Banned
	u.Banned = u.FailCount >= s.settings.MaxVerifyFail()
	if err := s.store.UpdateUser(u.ID, map[string]any{"fail_count": u.FailCount, "banned": u.Banned}); err != nil {
		return err
	}
	if u.Banned != wasBanned {
		s.events.userChanged(ctx, u, false)
	}
	return nil
}

func (s *UserService) SetBanned(ctx context.Context, u *model.User, banned bool) error {
	fields := map[string]any{"banned": banned}
	if !banned {
		fields["fail_count"] = 0
	}
	if err := s.store.UpdateUser(u.ID, fields); err != nil {
		return err
	}
	u.Banned = banned
	if !banned {
		u.FailCount = 0
	}
	s.events.userChanged(ctx, u, false)
	return nil
}

// ResetUnreplied 清零未回复计数（管理员回复或手动放行时）
func (s *UserService) ResetUnreplied(u *model.User) error {
	u.Unreplied, u.LimitNotified = 0, false
	return s.store.UpdateUser(u.ID, map[string]any{"unreplied": 0, "limit_notified": false})
}

func (s *UserService) MessageCount(userID int64) int64 {
	n, _ := s.store.CountMessages(userID)
	return n
}

type Stats struct {
	store.UserStats
	Messages int64
}

func (s *UserService) Stats() (Stats, error) {
	us, err := s.store.UserStats()
	if err != nil {
		return Stats{}, err
	}
	n, err := s.store.CountMessages(0)
	return Stats{UserStats: us, Messages: n}, err
}
