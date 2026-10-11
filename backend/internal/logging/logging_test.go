package logging

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 写日志与「订阅者退订」并发时不能 panic。
//
// 这里踩过一次 send on closed channel：分发原来在锁外做，而 Unsubscribe 会在锁里
// close(ch)，于是 SSE 客户端断开的瞬间正好有人在写日志，整个进程就挂在 panic 上
// （实测几毫秒内必现）。分发挪进锁里之后这个测试应当稳定通过。
func TestUnsubscribeDuringWrite(t *testing.T) {
	l, err := New("", LevelDebug, 128)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	done := make(chan struct{})
	var wg sync.WaitGroup
	worker := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					fn()
				}
			}
		}()
	}

	worker(func() { l.Infof("hello") })
	worker(func() {
		id, ch := l.Subscribe()
		// 顺手把通道排空，模拟真实的 SSE 消费端
		select {
		case <-ch:
		default:
		}
		l.Unsubscribe(id)
	})

	time.Sleep(200 * time.Millisecond)
	close(done)
	wg.Wait()
}

// 轮转之后日志要接着写在新文件里（打不开新文件时的退路见 rotateLocked）。
func TestRotateKeepsWriting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fpkstudio.log")
	l, err := New(p, LevelInfo, 16)
	if err != nil {
		t.Fatal(err)
	}
	l.Infof("before")

	// 把计数推到上限之上：下一条日志先落进当前文件，写完之后触发轮转。
	l.mu.Lock()
	l.fileSize = maxLogBytes + 1
	l.mu.Unlock()
	l.Infof("trigger")
	l.Infof("after")

	rotated, err := os.ReadFile(p + ".1")
	if err != nil {
		t.Fatalf("没有轮转出 %s.1: %v", p, err)
	}
	if !strings.Contains(string(rotated), "trigger") {
		t.Errorf("触发轮转的那条日志没留在轮转文件里: %q", rotated)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "after") {
		t.Errorf("轮转后的日志没写进新文件: %q", data)
	}
	if strings.Contains(string(data), "trigger") {
		t.Errorf("新文件里混进了轮转前的内容: %q", data)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
}
