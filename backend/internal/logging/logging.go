// Package logging 提供一个同时面向文件、内存环形缓冲和实时订阅者的日志器。
//
// 它取代了 fnpackup 里那套「在宿主目录 mkfifo 五根管道再阻塞读」的做法：
// 日志只有一个去处——数据目录下的文件，内存环形缓冲负责让 Web 端增量拉取，
// 订阅通道负责 SSE 实时推送。
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// 日志级别。
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// 日志来源。前端按这个字段把「应用日志」和「fnpack 打包输出」分开展示。
const (
	SourceApp    = "app"
	SourceFnpack = "fnpack"
	SourceBuild  = "build"
)

var levelRank = map[string]int{
	LevelDebug: 0,
	LevelInfo:  1,
	LevelWarn:  2,
	LevelError: 3,
}

const (
	maxLogBytes = 8 << 20 // 单文件 8MB
	maxBackups  = 3
)

// Entry 是一条日志。
type Entry struct {
	Seq    int64  `json:"seq"`
	Time   string `json:"time"`
	Level  string `json:"level"`
	Source string `json:"source"`
	Msg    string `json:"msg"`
}

// Logger 是所有组件的公共日志出口。
type Logger struct {
	mu       sync.Mutex
	ring     []Entry
	seq      int64
	file     *os.File
	fileSize int64
	minRank  int
	subs     map[int]chan Entry
	nextSub  int
}

// New 创建日志器。logFile 为空时只写内存。
func New(logFile, level string, ringSize int) (*Logger, error) {
	if ringSize <= 0 {
		ringSize = 5000
	}
	l := &Logger{
		ring:    make([]Entry, 0, ringSize),
		minRank: rankOf(level),
		subs:    make(map[int]chan Entry),
	}
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		l.file = f
		if st, err := f.Stat(); err == nil {
			l.fileSize = st.Size()
		}
	}
	return l, nil
}

// Close 关闭底层文件。
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

func rankOf(level string) int {
	if r, ok := levelRank[strings.ToLower(level)]; ok {
		return r
	}
	return levelRank[LevelInfo]
}

// Write 写一条日志。低于当前级别的条目会被丢弃。
func (l *Logger) Write(source, level, msg string) {
	if rankOf(level) < l.minRank {
		return
	}
	l.write(source, level, msg)
}

// Line 写一条不影响级别的日志（打包输出等原样转发），始终记录。
func (l *Logger) Line(source, level, msg string) {
	l.write(source, level, msg)
}

func (l *Logger) write(source, level, msg string) {
	msg = strings.TrimRight(msg, "\r\n")
	if msg == "" {
		return
	}
	now := time.Now()

	l.mu.Lock()
	l.seq++
	e := Entry{
		Seq:    l.seq,
		Time:   now.Format("2006-01-02 15:04:05.000"),
		Level:  level,
		Source: source,
		Msg:    msg,
	}
	l.ring = append(l.ring, e)
	if len(l.ring) > cap(l.ring) {
		// 丢掉最旧的一批，避免每次 append 都搬内存。
		drop := len(l.ring) - cap(l.ring)
		l.ring = append(l.ring[:0], l.ring[drop:]...)
	}
	l.writeFileLocked(now, source, level, msg)
	subs := make([]chan Entry, 0, len(l.subs))
	for _, ch := range l.subs {
		subs = append(subs, ch)
	}
	l.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- e:
		default: // 订阅者堵住就丢弃，绝不阻塞业务
		}
	}
}

func (l *Logger) writeFileLocked(t time.Time, source, level, msg string) {
	if l.file == nil {
		return
	}
	line := fmt.Sprintf("%s [%s] [%s] %s\n", t.Format("2006-01-02 15:04:05.000"), level, source, msg)
	n, err := l.file.WriteString(line)
	if err != nil {
		return
	}
	l.fileSize += int64(n)
	if l.fileSize > maxLogBytes {
		l.rotateLocked()
	}
}

func (l *Logger) rotateLocked() {
	name := l.file.Name()
	_ = l.file.Close()
	l.file = nil
	for i := maxBackups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", name, i), fmt.Sprintf("%s.%d", name, i+1))
	}
	_ = os.Rename(name, name+".1")
	if f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		l.file = f
		l.fileSize = 0
	}
}

// Snapshot 返回 seq > after 的日志，最多 limit 条（limit<=0 表示不限）。
// source 非空时只返回该来源。
func (l *Logger) Snapshot(after int64, limit int, source string) []Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]Entry, 0, 64)
	for _, e := range l.ring {
		if e.Seq <= after {
			continue
		}
		if source != "" && e.Source != source {
			continue
		}
		out = append(out, e)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// LastSeq 返回当前最大序号，用于前端初次连接时定位起点。
func (l *Logger) LastSeq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seq
}

// Subscribe 订阅实时日志。返回订阅 id 与只读通道。
func (l *Logger) Subscribe() (int, <-chan Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nextSub++
	id := l.nextSub
	ch := make(chan Entry, 256)
	l.subs[id] = ch
	return id, ch
}

// Unsubscribe 取消订阅并关闭通道。
func (l *Logger) Unsubscribe(id int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ch, ok := l.subs[id]; ok {
		delete(l.subs, id)
		close(ch)
	}
}

// ---- 便捷方法 ----

func (l *Logger) Debugf(format string, a ...any) {
	l.Write(SourceApp, LevelDebug, fmt.Sprintf(format, a...))
}
func (l *Logger) Infof(format string, a ...any) {
	l.Write(SourceApp, LevelInfo, fmt.Sprintf(format, a...))
}
func (l *Logger) Warnf(format string, a ...any) {
	l.Write(SourceApp, LevelWarn, fmt.Sprintf(format, a...))
}
func (l *Logger) Errorf(format string, a ...any) {
	l.Write(SourceApp, LevelError, fmt.Sprintf(format, a...))
}
