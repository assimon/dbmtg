package service

import (
	"context"
	"errors"
	"html/template"
	"path/filepath"
	"testing"
	"time"

	"github.com/assimon/dbmtg/internal/captcha"
	"github.com/assimon/dbmtg/internal/config"
	"github.com/assimon/dbmtg/internal/store"
)

type fakeProvider struct{ ok bool }

func (f *fakeProvider) Name() string                                         { return "fake" }
func (f *fakeProvider) Widget() template.HTML                                { return "" }
func (f *fakeProvider) Verify(context.Context, string, string) (bool, error) { return f.ok, nil }

func TestVerifyRules(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc := New(&config.Config{}, st, nil, []captcha.Provider{&fakeProvider{ok: false}}, nil)

	if m := svc.Settings.CaptchaMode(); m != "fake" {
		t.Fatalf("default mode = %q, want first provider", m)
	}
	u, _ := svc.Users.Upsert(1, "A", "", "")
	if svc.Users.Verified(u) {
		t.Fatal("new user should need verification")
	}

	// 失败累计，达到上限自动拉黑
	svc.Settings.SetMaxVerifyFail(3)
	for i, want := range []error{ErrVerifyFailed, ErrVerifyFailed, ErrBanned, ErrBanned} {
		if err := svc.Verify.Verify(context.Background(), 1, "tok", ""); !errors.Is(err, want) {
			t.Fatalf("attempt %d: err=%v want %v", i+1, err, want)
		}
	}
	svc.Users.SetBanned(u, false)
	if u, _ = svc.Users.Get(1); u.Banned || u.FailCount != 0 {
		t.Fatalf("unban should reset: %+v", u)
	}

	// 有效期
	svc.Users.MarkVerified(u)
	if !svc.Users.Verified(u) {
		t.Fatal("just verified")
	}
	svc.Settings.SetVerifyTTLDays(1)
	old := time.Now().Add(-48 * time.Hour)
	u.VerifiedAt = &old
	if svc.Users.Verified(u) {
		t.Fatal("verification should expire after ttl")
	}

	// 关闭验证后所有人视为已验证；不存在的 provider 回退 off
	svc.Settings.SetCaptchaMode(CaptchaOff)
	if !svc.Users.Verified(u) {
		t.Fatal("captcha off: everyone verified")
	}
	if err := svc.Settings.SetCaptchaMode("nope"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}
