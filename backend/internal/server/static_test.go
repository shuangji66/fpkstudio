package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

/*
静态资源的缓存头。

判据按「文件名会不会变」分两档（见 staticHandler）：`assets/` 下是 Vite 带内容哈希的
chunk，缓存一年、不再回源；其余（index.html 这类固定名字）每次都回源确认。这两条钉住
的理由是：写成 `Contains(name, "/assets/")` 时根目录那份产物根本匹配不上（`name` 是去掉
前导斜杠的相对路径），全部落进 no-cache——表现是「缓存头看着加了，资源却每次都在回源」，
而它只是一个字符串前缀的事，编译期什么都不会报。
*/
func TestStaticCacheHeaders(t *testing.T) {
	web := fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte("<html><head></head><body>hi</body></html>")},
		"assets/index-abc123.js":  &fstest.MapFile{Data: []byte("console.log(1)")},
		"assets/index-abc123.css": &fstest.MapFile{Data: []byte("body{}")},
		"favicon.txt":             &fstest.MapFile{Data: []byte("ico")},
	}

	cases := []struct {
		name     string
		path     string
		status   int
		cache    string
		ctype    string
		injected bool // 响应体是 index.html（带上注入的 base href 与运行时配置）
	}{
		{
			name:   "带哈希的 js 缓存一年",
			path:   "/assets/index-abc123.js",
			status: http.StatusOK,
			cache:  "public, max-age=31536000, immutable",
			ctype:  "text/javascript; charset=utf-8",
		},
		{
			name:   "带哈希的 css 也是一样",
			path:   "/assets/index-abc123.css",
			status: http.StatusOK,
			cache:  "public, max-age=31536000, immutable",
			ctype:  "text/css; charset=utf-8",
		},
		{
			name:     "首页每次回源",
			path:     "/",
			status:   http.StatusOK,
			cache:    "no-cache, no-store, must-revalidate",
			ctype:    "text/html; charset=utf-8",
			injected: true,
		},
		{
			name:     "SPA 兜底也回首页",
			path:     "/project/demo",
			status:   http.StatusOK,
			cache:    "no-cache, no-store, must-revalidate",
			ctype:    "text/html; charset=utf-8",
			injected: true,
		},
		{
			name:   "固定名字的资源要回源确认",
			path:   "/favicon.txt",
			status: http.StatusOK,
			cache:  "no-cache",
			ctype:  "text/plain; charset=utf-8",
		},
		{
			// 带后缀的路径不兜底：兜底成 200 text/html 的话前端只能从「不是 JSON」里猜
			name:   "不存在的资源给 404",
			path:   "/nope.png",
			status: http.StatusNotFound,
		},
		{
			name:   "没注册的接口不回 index.html",
			path:   "/api/nope",
			status: http.StatusNotFound,
		},
	}

	s, _ := newTestServerWeb(t, "/", web)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.status {
				t.Fatalf("状态码 = %d，想要 %d（body: %s）", rec.Code, tc.status, rec.Body.String())
			}
			if tc.cache == "" {
				return
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.cache {
				t.Errorf("Cache-Control = %q，想要 %q", got, tc.cache)
			}
			if got := rec.Header().Get("Content-Type"); got != tc.ctype {
				t.Errorf("Content-Type = %q，想要 %q", got, tc.ctype)
			}
			// 注入过才是「能跑起来的那份 index.html」：base href 决定资源挂在哪个路径下
			if tc.injected && !strings.Contains(rec.Body.String(), `<base href="/">`) {
				t.Errorf("index.html 没有注入 base href：%s", rec.Body.String())
			}
		})
	}
}

// 挂在子路径下时（baseURL=/app/）也要认得出来，别让前缀处理把 assets 的判据再弄丢一次。
func TestStaticCacheHeadersUnderBaseURL(t *testing.T) {
	web := fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html><head></head><body>hi</body></html>")},
		"assets/index-abc123.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}

	s, _ := newTestServerWeb(t, "/app", web)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/assets/index-abc123.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，想要 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q，想要一年 immutable", got)
	}

	// 兜底回的那份 index.html 要把 base href 指到实际挂载点，资源才找得到
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/project/demo", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<base href="/app/">`) {
		t.Errorf("兜底页面 = %d，base href 注入对不上：%s", rec.Code, rec.Body.String())
	}
}
