package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
	"github.com/assimon/dbmtg/internal/telegram"
)

const (
	maxPendingPerUser = 20
	receiptEmoji      = "👌"
)

// Outcome 用户消息的处理结果
type Outcome int

const (
	Delivered  Outcome = iota // 已转发到话题
	Banned                    // 已拉黑，静默丢弃
	NeedAck                   // 需先确认须知，消息已暂存
	NeedVerify                // 需先人机验证，消息已暂存
	Dropped                   // 超出未回复上限，已丢弃
)

var ErrNotRetractable = errors.New("只能撤回发给用户的消息")

// RelayService 负责用户私聊与管理群话题之间的双向消息中继
type RelayService struct {
	tg       *telegram.Client
	store    *store.Store
	users    *UserService
	settings *SettingService
	events   *Events
	groupID  int64

	// ponytail: 全局锁防止并发重复建话题 / 重复投递暂存消息，用户量大时可换成按用户加锁
	topicMu sync.Mutex
	flushMu sync.Mutex
}

// FromUser 处理用户发来的消息，依次检查：拉黑 → 须知确认 → 人机验证 → 未回复上限
func (s *RelayService) FromUser(ctx context.Context, u *model.User, msg *models.Message) (Outcome, error) {
	switch {
	case u.Banned:
		return Banned, nil
	case !s.users.Acked(u):
		return NeedAck, s.store.AddPending(u.ID, msg.ID, maxPendingPerUser)
	case !s.users.Verified(u):
		return NeedVerify, s.store.AddPending(u.ID, msg.ID, maxPendingPerUser)
	}
	s.FlushPending(ctx, u) // 刚通过验证/确认，或验证被关闭时，先补发暂存消息
	replyTo := 0
	if msg.ReplyToMessage != nil {
		replyTo = msg.ReplyToMessage.ID
	}
	return s.deliver(ctx, u, msg.ID, replyTo, msg.MediaGroupID)
}

// FlushPending 投递用户验证前暂存的消息（同样受未回复上限约束）
func (s *RelayService) FlushPending(ctx context.Context, u *model.User) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	ps, err := s.store.ListPending(u.ID)
	if err != nil {
		slog.Error("list pending", "user", u.ID, "err", err)
		return
	}
	for _, p := range ps {
		if _, err := s.deliver(ctx, u, p.MsgID, 0, ""); err != nil {
			slog.Warn("flush pending", "user", u.ID, "err", err)
		}
		s.store.DeletePending(p.ID)
	}
}

// deliver 检查未回复上限后转发，并处理送达回执和离开模式
func (s *RelayService) deliver(ctx context.Context, u *model.User, msgID, replyTo int, mediaGroup string) (Outcome, error) {
	counted := mediaGroup == "" || mediaGroup != u.LastMediaGroup // 同一相册只计一次
	if limit := s.settings.MaxUnreplied(); limit > 0 && counted && u.Unreplied >= limit {
		if !u.LimitNotified {
			u.LimitNotified = true
			if err := s.store.UpdateUser(u.ID, map[string]any{"limit_notified": true}); err != nil {
				return Dropped, err
			}
			s.events.limitReached(ctx, u, limit)
		}
		return Dropped, nil
	}

	if err := s.toGroup(ctx, u, msgID, replyTo); err != nil {
		return Delivered, err
	}
	if counted {
		u.Unreplied++
	}
	u.LastMediaGroup = mediaGroup
	// ponytail: 计数按内存值覆盖写，同一用户并发投递时可能少计，需要精确时改为 SQL 自增
	s.store.UpdateUser(u.ID, map[string]any{"unreplied": u.Unreplied, "last_media_group": mediaGroup})

	if s.settings.DeliveryReceipt() {
		s.tg.React(ctx, u.ID, msgID, receiptEmoji)
	}
	s.awayReply(ctx, u)
	return Delivered, nil
}

// awayReply 离开模式下自动回复，同一用户间隔 AwayHours 小时
func (s *RelayService) awayReply(ctx context.Context, u *model.User) {
	if !s.settings.AwayEnabled() {
		return
	}
	interval := time.Duration(s.settings.AwayHours()) * time.Hour
	if u.AwayNotifiedAt != nil && time.Since(*u.AwayNotifiedAt) < interval {
		return
	}
	if err := s.tg.SendStored(ctx, u.ID, s.settings.Away(), nil); err != nil {
		slog.Warn("away reply", "user", u.ID, "err", err)
		return
	}
	now := time.Now()
	u.AwayNotifiedAt = &now
	s.store.UpdateUser(u.ID, map[string]any{"away_notified_at": now})
}

func (s *RelayService) toGroup(ctx context.Context, u *model.User, msgID, replyToUserMsgID int) error {
	topic, err := s.ensureTopic(ctx, u)
	if err != nil {
		return err
	}
	p := &bot.CopyMessageParams{ChatID: s.groupID, MessageThreadID: topic, FromChatID: u.ID, MessageID: msgID}
	if replyToUserMsgID != 0 {
		if m := s.store.MsgByUserSide(u.ID, replyToUserMsgID); m != nil {
			p.ReplyParameters = &models.ReplyParameters{MessageID: m.GroupMsgID, AllowSendingWithoutReply: true}
		}
	}
	res, err := s.tg.Copy(ctx, p)
	if telegram.IsThreadNotFound(err) {
		// 话题被手动删除：重建后重发
		if err := s.store.UpdateUser(u.ID, map[string]any{"topic_id": 0, "header_msg_id": 0}); err != nil {
			return err
		}
		u.TopicID, u.HeaderMsgID = 0, 0
		if p.MessageThreadID, err = s.ensureTopic(ctx, u); err != nil {
			return err
		}
		p.ReplyParameters = nil
		res, err = s.tg.Copy(ctx, p)
	}
	if err != nil {
		return err
	}
	return s.store.SaveMsgMap(&model.MsgMap{UserID: u.ID, UserMsgID: msgID, GroupMsgID: res.ID})
}

func (s *RelayService) ensureTopic(ctx context.Context, u *model.User) (int, error) {
	s.topicMu.Lock()
	defer s.topicMu.Unlock()
	// 重新读库，避免其它 goroutine 已经建好
	if fresh, err := s.store.GetUser(u.ID); err == nil && fresh != nil && fresh.TopicID != 0 {
		u.TopicID, u.HeaderMsgID = fresh.TopicID, fresh.HeaderMsgID
		return u.TopicID, nil
	}
	id, err := s.tg.CreateTopic(ctx, s.groupID, TopicName(u))
	if err != nil {
		return 0, fmt.Errorf("create topic: %w", err)
	}
	if err := s.store.UpdateUser(u.ID, map[string]any{"topic_id": id}); err != nil {
		return 0, err
	}
	u.TopicID = id
	s.events.topicCreated(ctx, u)
	return id, nil
}

// RenameTopic 按最新资料更新话题标题
func (s *RelayService) RenameTopic(ctx context.Context, u *model.User) error {
	if u.TopicID == 0 {
		return nil
	}
	_, err := s.tg.EditForumTopic(ctx, &bot.EditForumTopicParams{ChatID: s.groupID, MessageThreadID: u.TopicID, Name: TopicName(u)})
	return err
}

// TopicName 话题标题：名字 @username | id
func TopicName(u *model.User) string {
	name := DisplayName(u.FirstName, u.LastName)
	if u.Username != "" {
		name += " @" + u.Username
	}
	if r := []rune(name); len(r) > 100 {
		name = string(r[:100])
	}
	return fmt.Sprintf("%s | %d", name, u.ID)
}

func DisplayName(first, last string) string {
	if n := strings.TrimSpace(first + " " + last); n != "" {
		return n
	}
	return "无名氏"
}

func (s *RelayService) signature(from *models.User) string {
	if !s.settings.ReplySignature() || from == nil {
		return ""
	}
	return "\n\n— " + DisplayName(from.FirstName, from.LastName)
}

// ToUser 把管理群话题内的消息转发给用户，成功后清零未回复计数
func (s *RelayService) ToUser(ctx context.Context, u *model.User, msg *models.Message) error {
	var reply *models.ReplyParameters
	if msg.ReplyToMessage != nil {
		if m := s.store.MsgByGroupSide(msg.ReplyToMessage.ID); m != nil {
			reply = &models.ReplyParameters{MessageID: m.UserMsgID, AllowSendingWithoutReply: true}
		}
	}
	sig := s.signature(msg.From)

	var userMsgID int
	if sig != "" && msg.Text != "" {
		// 署名追加在末尾，不影响原有 entities 的偏移
		m, err := s.tg.Send(ctx, &bot.SendMessageParams{
			ChatID: u.ID, Text: msg.Text + sig, Entities: msg.Entities, ReplyParameters: reply,
		})
		if err != nil {
			return err
		}
		userMsgID = m.ID
	} else {
		p := &bot.CopyMessageParams{ChatID: u.ID, FromChatID: msg.Chat.ID, MessageID: msg.ID, ReplyParameters: reply}
		if sig != "" && hasCaption(msg) {
			p.Caption, p.CaptionEntities = strings.TrimLeft(msg.Caption+sig, "\n"), msg.CaptionEntities
		}
		m, err := s.tg.Copy(ctx, p)
		if err != nil {
			return err
		}
		userMsgID = m.ID
	}
	if err := s.store.SaveMsgMap(&model.MsgMap{UserID: u.ID, UserMsgID: userMsgID, GroupMsgID: msg.ID, ToUser: true}); err != nil {
		return err
	}
	return s.users.ResetUnreplied(u)
}

// EditFromUser 用户编辑了已转发的消息，同步到话题
func (s *RelayService) EditFromUser(ctx context.Context, u *model.User, msg *models.Message) error {
	m := s.store.MsgByUserSide(u.ID, msg.ID)
	if m == nil || m.ToUser {
		return nil
	}
	return s.syncEdit(ctx, s.groupID, m.GroupMsgID, msg, "")
}

// EditFromAdmin 管理员编辑了发给用户的消息，同步到用户
func (s *RelayService) EditFromAdmin(ctx context.Context, u *model.User, msg *models.Message) error {
	m := s.store.MsgByGroupSide(msg.ID)
	if m == nil || !m.ToUser {
		return nil
	}
	return s.syncEdit(ctx, u.ID, m.UserMsgID, msg, s.signature(msg.From))
}

// syncEdit 同步文字和说明文字的修改。
// ponytail: 不同步替换媒体本身（需 editMessageMedia + InputMedia），有需要再加
func (s *RelayService) syncEdit(ctx context.Context, chatID int64, targetID int, msg *models.Message, sig string) error {
	var err error
	switch {
	case msg.Text != "":
		_, err = s.tg.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID: chatID, MessageID: targetID, Text: msg.Text + sig, Entities: msg.Entities,
		})
	case hasCaption(msg):
		_, err = s.tg.EditMessageCaption(ctx, &bot.EditMessageCaptionParams{
			ChatID: chatID, MessageID: targetID, Caption: strings.TrimLeft(msg.Caption+sig, "\n"), CaptionEntities: msg.CaptionEntities,
		})
	}
	if telegram.IsNotModified(err) {
		return nil
	}
	return err
}

// Retract 撤回管理员发给用户的消息，groupMsgID 为话题内的原消息
func (s *RelayService) Retract(ctx context.Context, u *model.User, groupMsgID int) error {
	m := s.store.MsgByGroupSide(groupMsgID)
	if m == nil || !m.ToUser || m.UserID != u.ID {
		return ErrNotRetractable
	}
	_, err := s.tg.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: u.ID, MessageID: m.UserMsgID})
	return err
}

// hasCaption 消息类型是否支持 caption
func hasCaption(m *models.Message) bool {
	return m.Photo != nil || m.Video != nil || m.Document != nil || m.Audio != nil || m.Animation != nil || m.Voice != nil
}
