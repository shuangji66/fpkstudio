package server

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// staticHandler 返回静态资源处理器。
func (s *Server) staticHandler() http.Handler {
	index, _ := fs.ReadFile(s.web, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, s.cfg.BaseURL)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			s.serveIndex(w, index)
			return
		}

		name := path.Clean(rel)
		if strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}

		data, err := fs.ReadFile(s.web, name)
		if err != nil {
			// SPA 兜底：只有「看起来不是资源请求」的路径才回 index.html。
			if !strings.Contains(path.Base(name), ".") {
				s.serveIndex(w, index)
				return
			}
			http.NotFound(w, r)
			return
		}

		if strings.Contains(name, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		w.Header().Set("Content-Type", contentTypeOf(name))
		http.ServeContent(w, r, name, s.startedAt, bytes.NewReader(data))
	})
}

// serveIndex 输出注入过 base href 与运行时配置的 index.html。
//
// 注入用「插入」而不是「替换占位符」：占位注释有可能被构建工具优化掉，
// 而 <head> 一定存在。同一份前端产物因此可以挂在任意 baseURL 下。
func (s *Server) serveIndex(w http.ResponseWriter, raw []byte) {
	base := s.cfg.BaseURL
	href := base
	if href != "/" {
		href += "/"
	}

	head := []byte("<head>")
	inject := []byte(`<head><base href="` + href + `">` +
		`<script>window.__FPKSTUDIO__={"baseURL":` + jsonString(base) +
		`,"version":` + jsonString(s.cfg.Version) + `};</script>`)

	body := raw
	if i := bytes.Index(raw, head); i >= 0 {
		body = make([]byte, 0, len(raw)+len(inject))
		body = append(body, raw[:i+len(head)]...)
		body = append(body, inject[len(head):]...)
		body = append(body, raw[i+len(head):]...)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Expires", "0")
	_, _ = w.Write(body)
}

func contentTypeOf(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json", ".map":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".ico":
		return "image/x-icon"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

// copyBody 把响应体交给 w，同时保证 Content-Length 已知。
func copyBody(w io.Writer, r io.Reader) error {
	_, err := io.Copy(w, r)
	return err
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '<':
			b.WriteString(`\u003c`)
		case '>':
			b.WriteString(`\u003e`)
		case '&':
			b.WriteString(`\u0026`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
