// Package model 定义持久化的数据模型。
package model

import "time"

// User 通过机器人私聊的访客
type User struct {
	ID           int64 `gorm:"primaryKey;autoIncrement:false"` // Telegram user id
	FirstName    string
	LastName     string
	Username     string
	LanguageCode string
	IsPremium    bool

	// 资料卡信息，建话题或手动刷新时从 getChat 拉取
	Bio             string
	Birthday        string // MM-DD 或 YYYY-MM-DD
	PersonalChannel string // 个人频道 @username 或标题

	TopicID     int `gorm:"index"` // 管理群中对应的话题，0 表示未创建
	HeaderMsgID int // 话题内置顶的资料卡消息

	VerifiedAt *time.Time
	AckedAt    *time.Time // 确认已阅读须知的时间
	Banned     bool
	FailCount  int // 连续验证失败次数

	Unreplied      int    // 管理员上次回复后已转发的消息数（相册算一条）
	LastMediaGroup string // 最近一条转发消息的相册 ID，用于相册只计一次
	LimitNotified  bool   // 本轮已提示过超出未回复上限

	AwayNotifiedAt *time.Time // 最近一次离开模式自动回复的时间

	CreatedAt time.Time
	UpdatedAt time.Time
}

// MsgMap 用户私聊消息与管理群消息的对应关系，用于双向引用回复、编辑同步和撤回
type MsgMap struct {
	ID         uint  `gorm:"primaryKey"`
	UserID     int64 `gorm:"index:idx_user_msg"`
	UserMsgID  int   `gorm:"index:idx_user_msg"`
	GroupMsgID int   `gorm:"index"`
	ToUser     bool  // true：管理员发给用户；false：用户发给管理群
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

// StoredMessage 管理员配置的一条消息（如须知、离开回复）。
// 优先用 copyMessage 复制 ChatID/MsgID 指向的原消息，保留格式和媒体；
// 原消息不可用时回退为纯文本 Text。
type StoredMessage struct {
	Text   string `json:"text,omitempty"`
	ChatID int64  `json:"chat_id,omitempty"`
	MsgID  int    `json:"msg_id,omitempty"`
}

// All 返回需要自动迁移的全部模型
func All() []any { return []any{&User{}, &MsgMap{}, &Pending{}, &Setting{}} }
