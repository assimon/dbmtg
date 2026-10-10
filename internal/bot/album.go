package bot

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
)

// albumWait 相册各条消息是分开推送的，收到第一条后等这么久再整组处理
var albumWait = 5 * time.Second

// albums 按相册攒消息。
// ponytail: 整组在定时器 goroutine 里处理，等待期间同一会话的后续单条消息可能先于相册送达，需要严格保序时改为在更新循环里冲刷
type albums struct {
	mu sync.Mutex
	m  map[string][]*models.Message
}

// add 收集相册消息；同一相册首条到达 albumWait 后，以整组调用 flush
func (a *albums) add(msg *models.Message, flush func([]*models.Message)) {
	key := fmt.Sprint(msg.Chat.ID, ":", msg.MediaGroupID)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.m == nil {
		a.m = map[string][]*models.Message{}
	}
	if _, ok := a.m[key]; !ok {
		time.AfterFunc(albumWait, func() {
			a.mu.Lock()
			msgs := a.m[key]
			delete(a.m, key)
			a.mu.Unlock()
			flush(msgs)
		})
	}
	a.m[key] = append(a.m[key], msg)
}
