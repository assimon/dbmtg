package store

import "github.com/assimon/dbmtg/internal/model"

func (s *Store) GetUser(id int64) (*model.User, error) {
	return first[model.User](s.db.Where("id = ?", id))
}

func (s *Store) UserByTopic(topicID int) (*model.User, error) {
	return first[model.User](s.db.Where("topic_id = ?", topicID))
}

func (s *Store) CreateUser(u *model.User) error {
	return s.db.Create(u).Error
}

// UpdateUser 只更新指定字段
func (s *Store) UpdateUser(id int64, fields map[string]any) error {
	return s.db.Model(&model.User{}).Where("id = ?", id).Updates(fields).Error
}

type UserStats struct {
	Total, Verified, Banned int64
}

func (s *Store) UserStats() (st UserStats, err error) {
	if err = s.db.Model(&model.User{}).Count(&st.Total).Error; err != nil {
		return
	}
	if err = s.db.Model(&model.User{}).Where("verified_at IS NOT NULL").Count(&st.Verified).Error; err != nil {
		return
	}
	err = s.db.Model(&model.User{}).Where("banned = ?", true).Count(&st.Banned).Error
	return
}
