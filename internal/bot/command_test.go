package bot

import (
	"reflect"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestArgsEntities(t *testing.T) {
	// "⚠️" 占 2 个 UTF-16 码元（U+26A0 + U+FE0F），"🟠" 占 2 个（代理对）
	text := "/setwelcome  ⚠️私聊 **注意**🟠 "
	name, args := parseCommand(text)
	if name != "setwelcome" || args != "⚠️私聊 **注意**🟠" {
		t.Fatalf("parse: %q %q", name, args)
	}
	ents := []models.MessageEntity{
		{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 11},
		{Type: models.MessageEntityTypeBold, Offset: 18, Length: 6},                            // "**注意**"
		{Type: models.MessageEntityTypeCustomEmoji, Offset: 24, Length: 2, CustomEmojiID: "1"}, // "🟠"
	}
	want := []models.MessageEntity{
		{Type: models.MessageEntityTypeBold, Offset: 5, Length: 6},
		{Type: models.MessageEntityTypeCustomEmoji, Offset: 11, Length: 2, CustomEmojiID: "1"},
	}
	if got := argsEntities(text, args, ents); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	// 命令后直接换行
	text = "/setwelcome\n🟠 hi"
	if name, args = parseCommand(text); name != "setwelcome" || args != "🟠 hi" {
		t.Fatalf("newline parse: %q %q", name, args)
	}
	ents = []models.MessageEntity{{Type: models.MessageEntityTypeCustomEmoji, Offset: 12, Length: 2, CustomEmojiID: "1"}}
	want = []models.MessageEntity{{Type: models.MessageEntityTypeCustomEmoji, Offset: 0, Length: 2, CustomEmojiID: "1"}}
	if got := argsEntities(text, args, ents); !reflect.DeepEqual(got, want) {
		t.Fatalf("newline: got %+v want %+v", got, want)
	}
}
