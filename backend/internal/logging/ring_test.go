package logging

import (
	"testing"
)

// 环缓冲、按 seq 增量取、来源过滤、级别过滤——这几条是前端「日志弹窗」的地基
// （见 stores/logs.ts 的合并逻辑），以前没有测试。

func TestSnapshotAfterLimitSource(t *testing.T) {
	l, err := New("", "debug", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	for i := 0; i < 5; i++ {
		l.Line(SourceApp, LevelInfo, "app-"+string(rune('a'+i)))
	}
	l.Line(SourceFnpack, LevelInfo, "fnpack-1")

	all := l.Snapshot(0, 0, "")
	if len(all) != 6 {
		t.Fatalf("全部日志 = %d 条，期望 6", len(all))
	}
	// after：只要 seq 更大的
	after := l.Snapshot(all[3].Seq, 0, "")
	if len(after) != 2 {
		t.Fatalf("after=%d 得到 %d 条，期望 2", all[3].Seq, len(after))
	}
	// limit：取最后 N 条
	limited := l.Snapshot(0, 2, "")
	if len(limited) != 2 || limited[0].Seq != all[4].Seq {
		t.Fatalf("limit=2 应当取最后两条，得到 %+v", limited)
	}
	// source：只取某个来源
	fnpackOnly := l.Snapshot(0, 0, SourceFnpack)
	if len(fnpackOnly) != 1 || fnpackOnly[0].Msg != "fnpack-1" {
		t.Fatalf("按来源过滤失败：%+v", fnpackOnly)
	}
	if l.LastSeq() != all[len(all)-1].Seq {
		t.Errorf("LastSeq = %d，期望 %d", l.LastSeq(), all[len(all)-1].Seq)
	}
}

func TestRingDropsOldest(t *testing.T) {
	l, err := New("", "debug", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	for i := 0; i < 5; i++ {
		l.Line(SourceApp, LevelInfo, "m"+string(rune('0'+i)))
	}
	snap := l.Snapshot(0, 0, "")
	if len(snap) != 3 {
		t.Fatalf("环大小 3 时应当只留 3 条，得到 %d", len(snap))
	}
	if snap[0].Msg != "m2" || snap[2].Msg != "m4" {
		t.Errorf("应当丢掉最旧的，得到 %q … %q", snap[0].Msg, snap[2].Msg)
	}
	// 丢掉旧的之后仍然接着收
	l.Line(SourceApp, LevelInfo, "m5")
	snap = l.Snapshot(0, 0, "")
	if snap[len(snap)-1].Msg != "m5" {
		t.Errorf("封顶之后仍然要接新日志：%+v", snap)
	}
}

func TestLevelFilterAndLine(t *testing.T) {
	l, err := New("", "warn", 10)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.Write(SourceApp, LevelDebug, "被级别挡掉")
	l.Write(SourceApp, LevelWarn, "保留")
	l.Line(SourceFnpack, LevelDebug, "打包输出原样转发")

	snap := l.Snapshot(0, 0, "")
	if len(snap) != 2 {
		t.Fatalf("期望 2 条，得到 %+v", snap)
	}
	if snap[0].Msg != "保留" || snap[1].Msg != "打包输出原样转发" {
		t.Errorf("级别过滤不对：%+v", snap)
	}
}

// 订阅者堵住时不能阻塞业务（分发是非阻塞的）。
func TestSlowSubscriberDoesNotBlock(t *testing.T) {
	l, err := New("", "debug", 100)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	id, ch := l.Subscribe()
	_ = id
	for i := 0; i < 1000; i++ {
		l.Line(SourceApp, LevelInfo, "spam")
	}
	// 通道有 256 的缓冲，写 1000 条不会把这里挂住；能走到这一行就说明没阻塞。
	l.Unsubscribe(id)
	if _, open := <-ch; open {
		// 关掉之后仍然可以读到缓冲里剩下的，读到通道关闭为止
		for range ch {
		}
	}
}
