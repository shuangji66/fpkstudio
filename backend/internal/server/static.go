package server

import (
	"bytes"
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

		// 没注册上的 /api/* 不回 SPA 兜底：回一份 index.html（200 text/html）时前端只能
		// 从「不是 JSON」里猜，而 404 + 一句人话能直接看出是路径写错了。
		if rel == "api" || strings.HasPrefix(rel, "api/") {
			writeError(w, notFound("没有这个接口：/%s", rel))
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

		// 前端产物分两类：`assets/` 下的文件名带内容哈希（Vite 生成的 chunk / css），
		// 内容一变名字就变，可以放心让浏览器缓存一年；其余（index.html、图标这类固定
		// 名字的文件）每次都得回源确认，不然升级之后用户拿到的还是旧页面。
		//
		// 判据用 `name` 这个**去掉前导斜杠**的相对路径（形状是 `assets/xxx.js`）。写成
		// Contains(name, "/assets/") 的话只有嵌套目录下的 assets 才认得出来，根目录这份
		// 会全部落进 no-cache——表现就是「缓存头加了，但资源每次都在回源」。
		if strings.HasPrefix(name, "assets/") {
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
