// Package telegram 封装 Telegram Bot API 客户端：统一的 429 重试和常用调用。
package telegram

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// AllowedUpdates 机器人订阅的更新类型
var AllowedUpdates = []string{"message", "callback_query"}

type Client struct {
	*bot.Bot
	WebhookSecret string
}

// New 创建客户端。更新按顺序串行处理，保证消息按序转发；
// 处理函数由接入层通过 RegisterHandlerMatchFunc 注册。
func New(token string) (*Client, error) {
	// webhook secret 由 token 派生，无需额外配置
	sum := sha256.Sum256([]byte("dbmtg-webhook:" + token))
	secret := hex.EncodeToString(sum[:16])
	b, err := bot.New(token,
		bot.WithNotAsyncHandlers(),
		bot.WithAllowedUpdates(AllowedUpdates),
		bot.WithWebhookSecretToken(secret),
		bot.WithDefaultHandler(func(context.Context, *bot.Bot, *models.Update) {}),
		bot.WithErrorsHandler(func(err error) { slog.Error("telegram", "err", err) }),
	)
	if err != nil {
		return nil, err
	}
	return &Client{Bot: b, WebhookSecret: secret}, nil
}

// Retry 遇到 429 时按 retry_after 等待后重试，最多 3 次
func Retry[T any](ctx context.Context, f func() (T, error)) (T, error) {
	for i := 0; ; i++ {
		v, err := f()
		var tm *bot.TooManyRequestsError
		if i >= 3 || !errors.As(err, &tm) {
			return v, err
		}
		select {
		case <-ctx.Done():
			return v, err
		case <-time.After(time.Duration(tm.RetryAfter+1) * time.Second):
		}
	}
}

// SendHTML 发送 HTML 格式消息，threadID 为 0 表示不指定话题
func (c *Client) SendHTML(ctx context.Context, chatID int64, threadID int, text string, markup models.ReplyMarkup) (*models.Message, error) {
	return c.Send(ctx, &bot.SendMessageParams{
		ChatID:             chatID,
		MessageThreadID:    threadID,
		Text:               text,
		ParseMode:          models.ParseModeHTML,
		ReplyMarkup:        markup,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	})
}

// Notify 发送 HTML 消息，失败只记日志
func (c *Client) Notify(ctx context.Context, chatID int64, threadID int, text string, markup models.ReplyMarkup) {
	if _, err := c.SendHTML(ctx, chatID, threadID, text, markup); err != nil {
		slog.Warn("send message", "chat", chatID, "thread", threadID, "err", err)
	}
}

func (c *Client) Send(ctx context.Context, p *bot.SendMessageParams) (*models.Message, error) {
	return Retry(ctx, func() (*models.Message, error) { return c.SendMessage(ctx, p) })
}

func (c *Client) Copy(ctx context.Context, p *bot.CopyMessageParams) (*models.MessageID, error) {
	return Retry(ctx, func() (*models.MessageID, error) { return c.CopyMessage(ctx, p) })
}

// CreateTopic 创建论坛话题，返回 message_thread_id
func (c *Client) CreateTopic(ctx context.Context, chatID int64, name string) (int, error) {
	t, err := Retry(ctx, func() (*models.ForumTopic, error) {
		return c.CreateForumTopic(ctx, &bot.CreateForumTopicParams{ChatID: chatID, Name: name})
	})
	if err != nil {
		return 0, err
	}
	return t.MessageThreadID, nil
}

// IsForbidden 对方屏蔽了机器人或账号已注销
func IsForbidden(err error) bool { return errors.Is(err, bot.ErrorForbidden) }

// IsThreadNotFound 话题已被删除
func IsThreadNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "thread not found")
}
