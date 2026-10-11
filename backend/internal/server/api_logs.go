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
