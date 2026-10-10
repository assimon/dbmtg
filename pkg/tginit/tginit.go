// Package tginit 校验 Telegram Mini App 的 initData。
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
package tginit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidHash = errors.New("tginit: invalid hash")
	ErrExpired     = errors.New("tginit: init data expired")
	ErrNoUser      = errors.New("tginit: missing user")
)

// User initData 中的用户信息
type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

// Validate 校验签名与时效，成功返回用户信息。maxAge <= 0 表示不检查时效。
func Validate(initData, botToken string, maxAge time.Duration, now time.Time) (*User, error) {
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return nil, err
	}
	hash := vals.Get("hash")
	vals.Del("hash")
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + vals.Get(k)
	}

	secret := Sign([]byte("WebAppData"), []byte(botToken))
	want := hex.EncodeToString(Sign(secret, []byte(strings.Join(lines, "\n"))))
	if !hmac.Equal([]byte(want), []byte(hash)) {
		return nil, ErrInvalidHash
	}
	authDate, _ := strconv.ParseInt(vals.Get("auth_date"), 10, 64)
	if maxAge > 0 && now.Sub(time.Unix(authDate, 0)) > maxAge {
		return nil, ErrExpired
	}
	var u User
	if err := json.Unmarshal([]byte(vals.Get("user")), &u); err != nil || u.ID == 0 {
		return nil, ErrNoUser
	}
	return &u, nil
}

// Sign 计算 HMAC-SHA256
func Sign(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
