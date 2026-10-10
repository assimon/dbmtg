package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
	"github.com/assimon/dbmtg/internal/telegram"
)

const maxPendingPerUser = 20

// RelayService 负责用户私聊与管理群话题之间的双向消息中继
type RelayService struct {
	tg          *telegram.Client
	store       *store.Store
	users       *UserService
	settings    *SettingService
	groupID     int64
	topicHeader func(*model.User) string

	// ponytail: 全局锁防止并发重复建话题 / 重复投递暂存消息，用户量大时可换成按用户加锁
	topicMu sync.Mutex
	flushMu sync.Mutex
}

// FromUser 处理用户发来的消息：未验证则暂存并返回 delivered=false，否则转发到话题
func (s *RelayService) FromUser(ctx context.Context, u *model.User, msg *models.Message) (delivered bool, err error) {
	if !s.users.Verified(u) {
		return false, s.store.AddPending(u.ID, msg.ID, maxPendingPerUser)
	}
	s.FlushPending(ctx, u) // 验证方式被关闭或刚验证通过时，先补发暂存消息
	replyTo := 0
	if msg.ReplyToMessage != nil {
		replyTo = msg.ReplyToMessage.ID
	}
	return true, s.toGroup(ctx, u, msg.ID, replyTo)
}

// FlushPending 投递用户验证前暂存的消息
func (s *RelayService) FlushPending(ctx context.Context, u *model.User) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	ps, err := s.store.ListPending(u.ID)
	if err != nil {
		slog.Error("list pending", "user", u.ID, "err", err)
		return
	}
	for _, p := range ps {
		if err := s.toGroup(ctx, u, p.MsgID, 0); err != nil {
			slog.Warn("flush pending", "user", u.ID, "err", err)
		}
		s.store.DeletePending(p.ID)
	}
}

func (s *RelayService) toGroup(ctx context.Context, u *model.User, msgID, replyToUserMsgID int) error {
	topic, err := s.ensureTopic(ctx, u)
	if err != nil {
		return err
	}
	p := &bot.CopyMessageParams{ChatID: s.groupID, MessageThreadID: topic, FromChatID: u.ID, MessageID: msgID}
	if replyToUserMsgID != 0 {
		if g := s.store.GroupMsgID(u.ID, replyToUserMsgID); g != 0 {
			p.ReplyParameters = &models.ReplyParameters{MessageID: g, AllowSendingWithoutReply: true}
		}
	}
	res, err := s.tg.Copy(ctx, p)
	if telegram.IsThreadNotFound(err) {
		// 话题被手动删除：重建后重发
		if err := s.store.UpdateUser(u.ID, map[string]any{"topic_id": 0}); err != nil {
			return err
		}
		u.TopicID = 0
		if p.MessageThreadID, err = s.ensureTopic(ctx, u); err != nil {
			return err
		}
		p.ReplyParameters = nil
		res, err = s.tg.Copy(ctx, p)
	}
	if err != nil {
		return err
	}
	return s.store.SaveMsgMap(u.ID, msgID, res.ID)
}

func (s *RelayService) ensureTopic(ctx context.Context, u *model.User) (int, error) {
	s.topicMu.Lock()
	defer s.topicMu.Unlock()
	// 重新读库，避免其它 goroutine 已经建好
	if fresh, err := s.store.GetUser(u.ID); err == nil && fresh != nil && fresh.TopicID != 0 {
		u.TopicID = fresh.TopicID
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
	if s.topicHeader != nil {
		s.tg.Notify(ctx, s.groupID, id, s.topicHeader(u), nil)
	}
	return id, nil
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

// ToUser 把管理群话题内的消息转发给用户，sender 为回复者（用于署名）
func (s *RelayService) ToUser(ctx context.Context, u *model.User, msg *models.Message) error {
	var reply *models.ReplyParameters
	if msg.ReplyToMessage != nil {
		if m := s.store.UserMsgID(msg.ReplyToMessage.ID); m != 0 {
			reply = &models.ReplyParameters{MessageID: m, AllowSendingWithoutReply: true}
		}
	}
	sig := ""
	if s.settings.ReplySignature() && msg.From != nil {
		sig = "\n\n— " + DisplayName(msg.From.FirstName, msg.From.LastName)
	}

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
	return s.store.SaveMsgMap(u.ID, userMsgID, msg.ID)
}

// hasCaption 消息类型是否支持 caption
func hasCaption(m *models.Message) bool {
	return m.Photo != nil || m.Video != nil || m.Document != nil || m.Audio != nil || m.Animation != nil || m.Voice != nil
}
