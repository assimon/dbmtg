package service

import (
	"time"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
)

type UserService struct {
	store    *store.Store
	settings *SettingService
}

// Upsert 创建或更新用户资料
func (s *UserService) Upsert(id int64, firstName, lastName, username string) (*model.User, error) {
	u, err := s.store.GetUser(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		u = &model.User{ID: id}
	}
	u.FirstName, u.LastName, u.Username = firstName, lastName, username
	return u, s.store.SaveUser(u)
}

func (s *UserService) Get(id int64) (*model.User, error) { return s.store.GetUser(id) }

func (s *UserService) ByTopic(topicID int) (*model.User, error) { return s.store.UserByTopic(topicID) }

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

func (s *UserService) SetBanned(u *model.User, banned bool) error {
	fields := map[string]any{"banned": banned}
	if !banned {
		fields["fail_count"] = 0
	}
	if err := s.store.UpdateUser(u.ID, fields); err != nil {
		return err
	}
	u.Banned = banned
	return nil
}

// MarkVerified 标记验证通过并清零失败次数
func (s *UserService) MarkVerified(u *model.User) error {
	now := time.Now()
	if err := s.store.UpdateUser(u.ID, map[string]any{"verified_at": now, "fail_count": 0}); err != nil {
		return err
	}
	u.VerifiedAt, u.FailCount = &now, 0
	return nil
}

// RecordVerifyFail 累计一次验证失败，达到上限自动拉黑
func (s *UserService) RecordVerifyFail(u *model.User) error {
	u.FailCount++
	u.Banned = u.FailCount >= s.settings.MaxVerifyFail()
	return s.store.UpdateUser(u.ID, map[string]any{"fail_count": u.FailCount, "banned": u.Banned})
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
