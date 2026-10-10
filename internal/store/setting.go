package store

import "github.com/assimon/dbmtg/internal/model"

// GetSetting 返回设置值，ok 为 false 表示未设置
func (s *Store) GetSetting(key string) (value string, ok bool) {
	st, _ := first[model.Setting](s.db.Where("key = ?", key))
	if st == nil {
		return "", false
	}
	return st.Value, true
}

func (s *Store) SetSetting(key, value string) error {
	return s.db.Save(&model.Setting{Key: key, Value: value}).Error
}
