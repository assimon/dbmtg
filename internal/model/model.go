// Package model 定义持久化的数据模型。
package model

import "time"

// User 通过机器人私聊的访客
type User struct {
	ID         int64 `gorm:"primaryKey;autoIncrement:false"` // Telegram user id
	FirstName  string
	LastName   string
	Username   string
	TopicID    int `gorm:"index"` // 管理群中对应的话题，0 表示未创建
	VerifiedAt *time.Time
	Banned     bool
	FailCount  int // 连续验证失败次数
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// MsgMap 用户私聊消息与管理群消息的对应关系，用于双向引用回复
type MsgMap struct {
	ID         uint  `gorm:"primaryKey"`
	UserID     int64 `gorm:"index:idx_user_msg"`
	UserMsgID  int   `gorm:"index:idx_user_msg"`
	GroupMsgID int   `gorm:"index"`
	CreatedAt  time.Time
}

// Pending 验证通过前暂存的用户消息
type Pending struct {
	ID        uint  `gorm:"primaryKey"`
	UserID    int64 `gorm:"index"`
	MsgID     int
	CreatedAt time.Time
}

// Setting 运行时可修改的键值设置
type Setting struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

// All 返回需要自动迁移的全部模型
func All() []any { return []any{&User{}, &MsgMap{}, &Pending{}, &Setting{}} }
