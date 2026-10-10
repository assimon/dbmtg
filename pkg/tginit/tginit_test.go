package tginit

import (
	"encoding/hex"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	const token = "123456:TEST-token"
	now := time.Unix(1_700_000_000, 0)
	user := `{"id":42,"first_name":"A"}`
	// data-check-string 手工按字母序拼接，独立于被测实现
	dcs := "auth_date=1700000000\nquery_id=Q1\nuser=" + user
	hash := hex.EncodeToString(Sign(Sign([]byte("WebAppData"), []byte(token)), []byte(dcs)))
	valid := url.Values{"query_id": {"Q1"}, "user": {user}, "auth_date": {"1700000000"}, "hash": {hash}}

	if u, err := Validate(valid.Encode(), token, time.Hour, now); err != nil || u.ID != 42 || u.FirstName != "A" {
		t.Fatalf("valid: user=%+v err=%v", u, err)
	}
	if _, err := Validate(valid.Encode(), "other:token", time.Hour, now); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("wrong token: %v", err)
	}
	tampered := url.Values{"query_id": {"Q1"}, "user": {`{"id":43}`}, "auth_date": {"1700000000"}, "hash": {hash}}
	if _, err := Validate(tampered.Encode(), token, time.Hour, now); !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("tampered: %v", err)
	}
	if _, err := Validate(valid.Encode(), token, time.Hour, now.Add(2*time.Hour)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired: %v", err)
	}
}
