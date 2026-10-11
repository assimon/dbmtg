package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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

// FromUser 处理用户发来的消息（单条或整个相册），依次检查：拉黑 → 须知确认 → 人机验证 → 未回复上限
func (s *RelayService) FromUser(ctx context.Context, u *model.User, msgs []*models.Message) (Outcome, error) {
	switch {
	case u.Banned:
		return Banned, nil
	case !s.users.Acked(u):
		return NeedAck, s.hold(u, msgs)
	case !s.users.Verified(u):
		return NeedVerify, s.hold(u, msgs)
	}
	s.FlushPending(ctx, u) // 刚通过验证/确认，或验证被关闭时，先补发暂存消息
	replyTo := 0
	if r := msgs[0].ReplyToMessage; r != nil {
		replyTo = r.ID
	}
	return s.deliver(ctx, u, msgIDs(msgs), replyTo)
}

// hold 暂存验证前的消息
func (s *RelayService) hold(u *model.User, msgs []*models.Message) error {
	for _, m := range msgs {
		if err := s.store.AddPending(u.ID, m.ID, maxPendingPerUser); err != nil {
			return err
		}
	}
	return nil
}

// FlushPending 投递用户验证前暂存的消息（同样受未回复上限约束）
// ponytail: 逐条投递，暂存的相册会拆散，需要时在 Pending 里记相册 ID 再分组
func (s *RelayService) FlushPending(ctx context.Context, u *model.User) {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	ps, err := s.store.ListPending(u.ID)
	if err != nil {
		slog.Error("list pending", "user", u.ID, "err", err)
		return
	}
	for _, p := range ps {
		if _, err := s.deliver(ctx, u, []int{p.MsgID}, 0); err != nil {
			slog.Warn("flush pending", "user", u.ID, "err", err)
		}
		s.store.DeletePending(p.ID)
	}
}

// deliver 检查未回复上限后转发，并处理送达回执和离开模式。一次投递（含整个相册）计一条
func (s *RelayService) deliver(ctx context.Context, u *model.User, msgIDs []int, replyTo int) (Outcome, error) {
	if limit := s.settings.MaxUnreplied(); limit > 0 && u.Unreplied >= limit {
		if !u.LimitNotified {
			u.LimitNotified = true
			if err := s.store.UpdateUser(u.ID, map[string]any{"limit_notified": true}); err != nil {
				return Dropped, err
			}
			s.events.limitReached(ctx, u, limit)
		}
		return Dropped, nil
	}

	if err := s.toGroup(ctx, u, msgIDs, replyTo); err != nil {
		return Delivered, err
	}
	u.Unreplied++
	// ponytail: 计数按内存值覆盖写，同一用户并发投递时可能少计，需要精确时改为 SQL 自增
	s.store.UpdateUser(u.ID, map[string]any{"unreplied": u.Unreplied})

	if s.settings.DeliveryReceipt() {
		s.tg.React(ctx, u.ID, msgIDs[0], receiptEmoji)
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

func (s *RelayService) toGroup(ctx context.Context, u *model.User, msgIDs []int, replyToUserMsgID int) error {
	topic, err := s.ensureTopic(ctx, u)
	if err != nil {
		return err
	}
	var reply *models.ReplyParameters
	if replyToUserMsgID != 0 {
		if m := s.store.MsgByUserSide(u.ID, replyToUserMsgID); m != nil {
			reply = &models.ReplyParameters{MessageID: m.GroupMsgID, AllowSendingWithoutReply: true}
		}
	}
	res, err := s.copy(ctx, s.groupID, topic, u.ID, msgIDs, reply)
	if telegram.IsTopicClosed(err) {
		// 话题被手动关闭：重新打开后重发
		if _, err := s.tg.ReopenForumTopic(ctx, &bot.ReopenForumTopicParams{ChatID: s.groupID, MessageThreadID: topic}); err != nil {
			return fmt.Errorf("reopen topic: %w", err)
		}
		res, err = s.copy(ctx, s.groupID, topic, u.ID, msgIDs, reply)
	}
	if telegram.IsThreadNotFound(err) {
		// 话题被手动删除：重建后重发
		if err := s.store.UpdateUser(u.ID, map[string]any{"topic_id": 0, "header_msg_id": 0}); err != nil {
			return err
		}
		u.TopicID, u.HeaderMsgID = 0, 0
		if topic, err = s.ensureTopic(ctx, u); err != nil {
			return err
		}
		res, err = s.copy(ctx, s.groupID, topic, u.ID, msgIDs, nil)
	}
	if err != nil {
		return err
	}
	return s.saveMaps(u.ID, msgIDs, res, false)
}

// copy 单条用 copyMessage（可带回复引用）；相册用 copyMessages 保持分组，该接口不支持回复引用
func (s *RelayService) copy(ctx context.Context, to int64, thread int, from int64, msgIDs []int, reply *models.ReplyParameters) ([]int, error) {
	if len(msgIDs) == 1 {
		m, err := s.tg.Copy(ctx, &bot.CopyMessageParams{ChatID: to, MessageThreadID: thread, FromChatID: from, MessageID: msgIDs[0], ReplyParameters: reply})
		if err != nil {
			return nil, err
		}
		return []int{m.ID}, nil
	}
	res, err := s.tg.CopyAll(ctx, &bot.CopyMessagesParams{ChatID: to, MessageThreadID: thread, FromChatID: from, MessageIDs: msgIDs})
	ids := make([]int, len(res))
	for i, m := range res {
		ids[i] = m.ID
	}
	return ids, err
}

// saveMaps 记录两侧消息的对应关系，userIDs 与 groupIDs 按位置一一对应
func (s *RelayService) saveMaps(userID int64, userIDs, groupIDs []int, toUser bool) error {
	for i := range min(len(userIDs), len(groupIDs)) {
		if err := s.store.SaveMsgMap(&model.MsgMap{UserID: userID, UserMsgID: userIDs[i], GroupMsgID: groupIDs[i], ToUser: toUser}); err != nil {
			return err
		}
	}
	return nil
}

// msgIDs 取消息 ID 并升序排列（copyMessages 要求）
func msgIDs(msgs []*models.Message) []int {
	ids := make([]int, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	slices.Sort(ids)
	return ids
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

// BindGroup 记录话题数据所属的管理群。管理群更换后，旧话题 ID 在新群里可能与新话题重号，
// 导致回复发错人，因此清空所有话题绑定和消息映射，用户下次发消息时自动在新群建话题。
func (s *RelayService) BindGroup() (reset bool, err error) {
	bound := fmt.Sprint(s.groupID)
	old, ok := s.store.GetSetting(keyBoundGroup)
	if ok && old == bound {
		return false, nil
	}
	if ok {
		if err := s.store.ResetTopics(); err != nil {
			return false, err
		}
	}
	return ok, s.store.SetSetting(keyBoundGroup, bound)
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

// ToUser 把管理群话题内的消息（单条或整个相册）转发给用户，成功后清零未回复计数
func (s *RelayService) ToUser(ctx context.Context, u *model.User, msgs []*models.Message) error {
	if len(msgs) > 1 {
		// ponytail: 相册整组复制，不加署名也不带回复引用；需要署名时可在复制后对带说明文字的那条 editMessageCaption
		groupIDs := msgIDs(msgs)
		userIDs, err := s.copy(ctx, u.ID, 0, msgs[0].Chat.ID, groupIDs, nil)
		if err != nil {
			return err
		}
		if err := s.saveMaps(u.ID, userIDs, groupIDs, true); err != nil {
			return err
		}
		return s.users.ResetUnreplied(u)
	}
	msg := msgs[0]
	var reply *models.ReplyParameters
	if msg.ReplyToMessage != nil {
		if m := s.store.MsgByGroupSide(u.ID, msg.ReplyToMessage.ID); m != nil {
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
	s.archiveBeforeEdit(ctx, u, m.GroupMsgID)
	return s.syncEdit(ctx, s.groupID, m.GroupMsgID, msg, "")
}

// archiveBeforeEdit 机器人改自己的消息不显示「已编辑」，同步前在话题里发提醒，并把修改前的版本复制一份留档
func (s *RelayService) archiveBeforeEdit(ctx context.Context, u *model.User, groupMsgID int) {
	n, err := s.tg.Send(ctx, &bot.SendMessageParams{
		ChatID: s.groupID, MessageThreadID: u.TopicID, Text: "✏️ 用户编辑了这条消息，修改前的内容如下：",
		ReplyParameters: &models.ReplyParameters{MessageID: groupMsgID, AllowSendingWithoutReply: true},
	})
	if err == nil {
		_, err = s.tg.Copy(ctx, &bot.CopyMessageParams{
			ChatID: s.groupID, MessageThreadID: u.TopicID, FromChatID: s.groupID, MessageID: groupMsgID,
			ReplyParameters: &models.ReplyParameters{MessageID: n.ID, AllowSendingWithoutReply: true}, DisableNotification: true,
		})
	}
	if err != nil {
		slog.Warn("archive before edit", "user", u.ID, "err", err)
	}
}

// EditFromAdmin 管理员编辑了发给用户的消息，同步到用户
func (s *RelayService) EditFromAdmin(ctx context.Context, u *model.User, msg *models.Message) error {
	m := s.store.MsgByGroupSide(u.ID, msg.ID)
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
	m := s.store.MsgByGroupSide(u.ID, groupMsgID)
	if m == nil || !m.ToUser {
		return ErrNotRetractable
	}
	_, err := s.tg.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: u.ID, MessageID: m.UserMsgID})
	return err
}

// hasCaption 消息类型是否支持 caption
func hasCaption(m *models.Message) bool {
	return m.Photo != nil || m.Video != nil || m.Document != nil || m.Audio != nil || m.Animation != nil || m.Voice != nil
}
