package bot

import (
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

func TestAlbums(t *testing.T) {
	albumWait = 20 * time.Millisecond
	var a albums
	got := make(chan []*models.Message, 2)
	flush := func(msgs []*models.Message) { got <- msgs }
	for i, g := range []string{"x", "x", "y", "x"} {
		a.add(&models.Message{ID: i, Chat: models.Chat{ID: 1}, MediaGroupID: g}, flush)
	}
	sizes := map[int]bool{}
	for range 2 {
		sizes[len(<-got)] = true
	}
	if !sizes[3] || !sizes[1] {
		t.Fatalf("group sizes = %v, want 3 and 1", sizes)
	}
}
