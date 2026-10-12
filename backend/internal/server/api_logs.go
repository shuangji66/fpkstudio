package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"fpkstudio/internal/logging"
)

type logQuery struct {
	after  int64
	limit  int
	source string
}

func parseLogQuery(r *http.Request) logQuery {
	q := r.URL.Query()
	lq := logQuery{limit: 500}
	if v, err := strconv.ParseInt(q.Get("after"), 10, 64); err == nil {
		lq.after = v
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		lq.limit = v
	}
	lq.source = q.Get("source")
	return lq
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) error {
	lq := parseLogQuery(r)
	entries := s.log.Snapshot(lq.after, lq.limit, lq.source)
	return writeJSON(w, map[string]any{
		"list":    entries,
		"lastSeq": s.log.LastSeq(),
		// 进程标识：seq 重启后会从 1 开始，前端要靠它把游标归零（见 logging.Logger.boot）。
		"boot": s.log.Boot(),
	})
}

// handleLogStream 用 SSE 推送实时日志。
//
// 先按 after 补发历史，再转成实时推送，前端 reconnect 时只要带上最后一条的
// seq 就不会丢也不会重复。
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return internal("当前连接不支持流式响应")
	}
	lq := parseLogQuery(r)

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	id, ch := s.log.Subscribe()
	defer s.log.Unsubscribe(id)

	// 第一帧先报「这是哪个进程」。前端的 seq 去重全靠它：后端重启后 seq 从 1 重新
	// 开始（EventSource 自带重连，用的还是旧的 after），不告诉它，新日志会被整片
	// 当成旧的丢掉——表现是「日志弹窗从此不再更新」。
	boot, err := json.Marshal(map[string]any{"boot": s.log.Boot(), "lastSeq": s.log.LastSeq()})
	if err != nil {
		return internal("序列化进程标识失败: %v", err)
	}
	if _, err := fmt.Fprintf(w, "event: boot\ndata: %s\n\n", boot); err != nil {
		return nil
	}
	flusher.Flush()

	for _, e := range s.log.Snapshot(lq.after, 0, lq.source) {
		if err := writeSSE(w, e); err != nil {
			return nil
		}
	}
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-s.closing:
			// 关服：Shutdown 不会取消请求的 context，挂着不动的 SSE 会让它等满超时。
			return nil
		case e, open := <-ch:
			if !open {
				return nil
			}
			if lq.source != "" && e.Source != lq.source {
				continue
			}
			if err := writeSSE(w, e); err != nil {
				return nil
			}
			flusher.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, e logging.Entry) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.Seq, body); err != nil {
		return err
	}
	return nil
}
