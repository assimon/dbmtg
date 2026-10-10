package store

import "github.com/assimon/dbmtg/internal/model"

func (s *Store) SaveMsgMap(m *model.MsgMap) error {
	return s.db.Create(m).Error
}

// MsgByUserSide 按用户侧消息 ID 查对应关系，找不到返回 nil
func (s *Store) MsgByUserSide(userID int64, userMsgID int) *model.MsgMap {
	m, _ := first[model.MsgMap](s.db.Where("user_id = ? AND user_msg_id = ?", userID, userMsgID))
	return m
}

// MsgByGroupSide 按管理群侧消息 ID 查对应关系，找不到返回 nil
func (s *Store) MsgByGroupSide(groupMsgID int) *model.MsgMap {
	m, _ := first[model.MsgMap](s.db.Where("group_msg_id = ?", groupMsgID))
	return m
}

// CountMessages 统计消息数，userID 为 0 时统计全部
func (s *Store) CountMessages(userID int64) (n int64, err error) {
	q := s.db.Model(&model.MsgMap{})
	if userID != 0 {
		q = q.Where("user_id = ?", userID)
	}
	err = q.Count(&n).Error
	return
}

// AddPending 暂存一条消息，每个用户最多保留 limit 条
func (s *Store) AddPending(userID int64, msgID, limit int) error {
	var n int64
	if err := s.db.Model(&model.Pending{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
		return err
	}
	if n >= int64(limit) {
		return nil
	}
	return s.db.Create(&model.Pending{UserID: userID, MsgID: msgID}).Error
}

func (s *Store) ListPending(userID int64) ([]model.Pending, error) {
	var ps []model.Pending
	err := s.db.Where("user_id = ?", userID).Order("id").Find(&ps).Error
	return ps, err
}

func (s *Store) DeletePending(id uint) error {
	return s.db.Delete(&model.Pending{}, id).Error
}
