package service

import (
	"context"
	"errors"
	"html/template"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/model"
	"github.com/assimon/dbmtg/internal/store"
)

type fakeProvider struct{ ok bool }

func (f *fakeProvider) Name() string                                         { return "fake" }
func (f *fakeProvider) Widget() template.HTML                                { return "" }
func (f *fakeProvider) Verify(context.Context, string, string) (bool, error) { return f.ok, nil }

func newTestServices(t *testing.T) *Services {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	// tg 为 nil：测试只覆盖不调用 Telegram 的路径
	return New(&config.Config{}, st, nil, []captcha.Provider{&fakeProvider{ok: false}})
}

func TestVerifyRules(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices(t)

	if m := svc.Settings.CaptchaMode(); m != "fake" {
		t.Fatalf("default mode = %q, want first provider", m)
	}
	u, _ := svc.Users.Upsert(ctx, &models.User{ID: 1, FirstName: "A"})
	if svc.Users.Verified(u) {
		t.Fatal("new user should need verification")
	}

	// 失败累计，达到上限自动拉黑
	svc.Settings.SetMaxVerifyFail(3)
	for i, want := range []error{ErrVerifyFailed, ErrVerifyFailed, ErrBanned, ErrBanned} {
		if err := svc.Verify.Verify(ctx, 1, "tok", ""); !errors.Is(err, want) {
			t.Fatalf("attempt %d: err=%v want %v", i+1, err, want)
		}
	}
	svc.Users.SetBanned(ctx, u, false)
	if u, _ = svc.Users.Get(1); u.Banned || u.FailCount != 0 {
		t.Fatalf("unban should reset: %+v", u)
	}

	// 有效期
	svc.Users.MarkVerified(ctx, u)
	if !svc.Users.Verified(u) {
		t.Fatal("just verified")
	}
	if err := svc.Verify.Verify(ctx, 1, "tok", ""); !errors.Is(err, ErrAlreadyVerified) {
		t.Fatalf("re-verify: err=%v want ErrAlreadyVerified", err)
	}
	svc.Settings.SetVerifyTTLDays(1)
	old := time.Now().Add(-48 * time.Hour)
	u.VerifiedAt = &old
	if svc.Users.Verified(u) {
		t.Fatal("verification should expire after ttl")
	}

	// 关闭验证后所有人视为已验证；不存在的 provider 拒绝
	svc.Settings.SetCaptchaMode(CaptchaOff)
	if !svc.Users.Verified(u) {
		t.Fatal("captcha off: everyone verified")
	}
	if err := svc.Settings.SetCaptchaMode("nope"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestFromUserGates(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices(t)
	u, _ := svc.Users.Upsert(ctx, &models.User{ID: 1, FirstName: "A"})
	msg := &models.Message{ID: 100}
	pending := func() int { ps, _ := svc.Relay.store.ListPending(1); return len(ps) }

	check := func(want Outcome) {
		t.Helper()
		if got, err := svc.Relay.FromUser(ctx, u, msg); err != nil || got != want {
			t.Fatalf("outcome=%v err=%v, want %v", got, err, want)
		}
	}

	// 须知确认优先于验证，消息暂存
	svc.Settings.SetWelcomeConfirm(true)
	check(NeedAck)
	svc.Users.MarkAcked(u)
	check(NeedVerify)
	if pending() != 2 {
		t.Fatalf("pending = %d, want 2", pending())
	}
	svc.Relay.store.DeletePending(1)
	svc.Relay.store.DeletePending(2)

	// 拉黑静默丢弃
	svc.Users.SetBanned(ctx, u, true)
	check(Banned)
	svc.Users.SetBanned(ctx, u, false)

	// 超出未回复上限：丢弃，事件本轮只触发一次，放行后重置
	svc.Settings.SetCaptchaMode(CaptchaOff)
	svc.Settings.SetMaxUnreplied(2)
	fired := 0
	svc.Events.LimitReached = func(context.Context, *model.User, int) { fired++ }
	u.Unreplied = 2
	check(Dropped)
	check(Dropped)
	if fired != 1 {
		t.Fatalf("LimitReached fired %d times, want 1", fired)
	}
	svc.Users.ResetUnreplied(u)
	if got, _ := svc.Users.Get(1); got.Unreplied != 0 || got.LimitNotified {
		t.Fatalf("reset failed: %+v", got)
	}
}

func TestStoredMessageSettings(t *testing.T) {
	svc := newTestServices(t)
	if !reflect.DeepEqual(svc.Settings.Welcome(), DefaultWelcome) {
		t.Fatal("expected default welcome")
	}
	want := model.StoredMessage{Text: "须知", ChatID: 9, MsgID: 42}
	if err := svc.Settings.SetWelcome(want); err != nil {
		t.Fatal(err)
	}
	if got := svc.Settings.Welcome(); !reflect.DeepEqual(got, want) {
		t.Fatalf("welcome = %+v, want %+v", got, want)
	}
	if err := svc.Settings.SetAway(model.StoredMessage{}); err == nil {
		t.Fatal("empty message accepted")
	}
}

func TestBindGroupResetsTopics(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices(t)
	st := svc.Relay.store
	u, _ := svc.Users.Upsert(ctx, &models.User{ID: 1, FirstName: "A"})
	st.UpdateUser(u.ID, map[string]any{"topic_id": 10, "header_msg_id": 11})
	st.SaveMsgMap(&model.MsgMap{UserID: 1, UserMsgID: 1, GroupMsgID: 2})

	svc.Relay.groupID = -100
	if reset, err := svc.Relay.BindGroup(); err != nil || reset {
		t.Fatalf("first bind: reset=%v err=%v", reset, err)
	}
	if reset, _ := svc.Relay.BindGroup(); reset {
		t.Fatal("same group should not reset")
	}
	svc.Relay.groupID = -200
	if reset, err := svc.Relay.BindGroup(); err != nil || !reset {
		t.Fatalf("changed group: reset=%v err=%v", reset, err)
	}
	if got, _ := st.GetUser(1); got.TopicID != 0 || got.HeaderMsgID != 0 {
		t.Fatalf("topic not reset: %+v", got)
	}
	if n, _ := st.CountMessages(0); n != 0 {
		t.Fatalf("msg maps not cleared: %d", n)
	}
}

// Upsert 只更新资料字段，不动话题等状态；改名触发 UserChanged
func TestUpsertProfileOnly(t *testing.T) {
	ctx := context.Background()
	svc := newTestServices(t)
	svc.Users.Upsert(ctx, &models.User{ID: 1, FirstName: "A"})
	svc.Relay.store.UpdateUser(1, map[string]any{"topic_id": 10})
	renamed := false
	svc.Events.UserChanged = func(_ context.Context, _ *model.User, r bool) { renamed = r }

	u, _ := svc.Users.Upsert(ctx, &models.User{ID: 1, FirstName: "B"})
	if got, _ := svc.Users.Get(1); got.TopicID != 10 || got.FirstName != "B" {
		t.Fatalf("state clobbered: %+v", got)
	}
	if !renamed || u.FirstName != "B" {
		t.Fatal("rename event not fired")
	}
}
