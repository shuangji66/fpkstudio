package server

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// 这一组钉住复审里「状态码语义 / 体积上限 / 静态资源注入」几条。

// TestFSErrorCodes：用户改一下就能过的错误不该回 500，也不该都挤到 404。
//
// 404 在前端是「这个文件还没创建」的正常信号（向导、生命周期脚本本来就可能没有），
// 权限和类型错误混进去会让用户以为打开的是一个空文件。
func TestFSErrorCodes(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)
	if err := os.WriteFile(filepath.Join(dataDir, "projects", "demo", "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		body any
		want int
	}{
		{"读取不存在的文件", "/api/fs/read", map[string]any{"path": "demo/nope.txt"}, http.StatusNotFound},
		{"读取一个目录", "/api/fs/read", map[string]any{"path": "demo"}, http.StatusBadRequest},
		{"写入越界路径", "/api/fs/write", map[string]any{"path": "../x", "content": "x"}, http.StatusBadRequest},
		{"删除根目录", "/api/fs/delete", map[string]any{"path": ""}, http.StatusBadRequest},
		{"把目录移动到自己里面", "/api/fs/rename", map[string]any{"path": "demo", "to": "demo/sub"}, http.StatusBadRequest},
		{"重命名到已存在的目标", "/api/fs/rename", map[string]any{"path": "demo/notes.txt", "to": "demo/manifest"}, http.StatusConflict},
		{"新建同名文件", "/api/fs/create", map[string]any{"path": "demo/notes.txt", "dir": false}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := doJSON(t, s, tc.path, tc.body)
			if code != tc.want {
				t.Errorf("状态码 = %d，期望 %d（%v）", code, tc.want, body)
			}
		})
	}
}

// TestMethodNotAllowed：方法不对要回 405，而不是「请求体不是合法 JSON: EOF」。
func TestMethodNotAllowed(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/projects/delete"},
		{http.MethodGet, "/api/fs/write"},
		{http.MethodPost, "/api/projects"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s 状态码 = %d，期望 405（%s）", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

// TestReadTooLarge：超过上限的文件只回元信息，不能把整份内容塞进 JSON。
//
// app/server 下几百 MB 的原生程序很常见，以前这个接口会先整份读进内存再决定要不要回。
func TestReadTooLarge(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)
	big := filepath.Join(proj, "big.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte("a"), 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	old := maxReadBytes
	maxReadBytes = 64
	t.Cleanup(func() { maxReadBytes = old })

	code, body := doJSON(t, s, "/api/fs/read", map[string]any{"path": "demo/big.bin"})
	if code != http.StatusOK {
		t.Fatalf("状态码 = %d（%v）", code, body)
	}
	if body["tooLarge"] != true {
		t.Errorf("tooLarge 应当为 true：%v", body)
	}
	if body["content"] != "" {
		t.Errorf("超限时不该带内容：%v", body)
	}
	if body["size"] != float64(4096) {
		t.Errorf("size = %v，期望 4096", body["size"])
	}
}

// TestUploadTooLarge：上传超过上限回 413（而不是把盘灌满）。
func TestUploadTooLarge(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)

	old := maxUploadBytes
	maxUploadBytes = 1024
	t.Cleanup(func() { maxUploadBytes = old })

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", "demo")
	fw, err := mw.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/fs/upload", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413（%s）", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "projects", "demo", "big.bin")); err == nil {
		t.Error("超限的请求不该留下文件")
	}
}

// TestUploadPathMatchesDisk：上传响应里的 path 要和真正落盘的位置一致
// （fsx.Rel 会去掉前导斜杠、归一 `..`，以前这里原样回 target + name）。
func TestUploadPathMatchesDisk(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("path", "/demo")
	fw, err := mw.CreateFormFile("file", "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("x"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/fs/upload", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d（%s）", rec.Code, rec.Body.String())
	}

	var out struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || out.Files[0].Path != "demo/b.txt" {
		t.Fatalf("响应里的路径 = %+v，期望 demo/b.txt", out.Files)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "projects", "demo", "b.txt")); err != nil {
		t.Errorf("文件没有落在响应说的位置: %v", err)
	}
}

// TestUploadModeFollowsServerDir：打包落地目录是可配置的，上传到那里的二进制要带执行位。
//
// 以前判据写死 app/server：用户把 serverDir 改成 app/bin 之后上传的程序是 0644，
// 产物装到机器上起不来。
func TestUploadModeFollowsServerDir(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)
	if err := os.MkdirAll(filepath.Join(dataDir, "projects", "demo", "app", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dataDir, "projects", "demo", "building", "settings.json"),
		[]byte("{\"multiPlatform\":false,\"platforms\":[],\"serverDir\":\"app/bin\",\"copyToOutput\":true}"),
		0o644,
	); err != nil {
		if err := os.MkdirAll(filepath.Join(dataDir, "projects", "demo", "building"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(dataDir, "projects", "demo", "building", "settings.json"),
			[]byte("{\"multiPlatform\":false,\"platforms\":[],\"serverDir\":\"app/bin\",\"copyToOutput\":true}"),
			0o644,
		); err != nil {
			t.Fatal(err)
		}
	}

	if mode := s.uploadMode("demo/app/bin/tool"); mode != 0o755 {
		t.Errorf("app/bin 下的上传模式 = %04o，期望 0755", mode)
	}
	if mode := s.uploadMode("demo/app/www/x.js"); mode != 0o644 {
		t.Errorf("其它目录 = %04o，期望 0644", mode)
	}
}

// TestIndexHTMLInjected：直接访问 index.html 也要带 base href 与运行时配置。
//
// 原样发出去的话，用 /app/fpkstudio/index.html 打开（书签、外链）会拿到一个没有
// window.__FPKSTUDIO__ 的页面，前端把 baseURL 回落成 "/"，所有 /api/* 都打错前缀。
func TestIndexHTMLInjected(t *testing.T) {
	web := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><head></head><body></body></html>")},
	}
	s, _ := newTestServerWeb(t, "/app/fpkstudio", fs.FS(web))

	for _, path := range []string{"/app/fpkstudio/", "/app/fpkstudio/index.html"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `window.__FPKSTUDIO__`) {
			t.Errorf("%s 没有注入运行时配置：%s", path, body)
		}
		if !strings.Contains(body, `<base href="/app/fpkstudio/">`) {
			t.Errorf("%s 没有注入 base href：%s", path, body)
		}
	}
}

// handler panic 不能把进程带走，也不该只留一行 stderr（日志弹窗看不到）。
func TestPanicIsRecovered(t *testing.T) {
	s, _ := newTestServer(t)
	s.api("/api/test/panic", func(http.ResponseWriter, *http.Request) error {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test/panic", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "服务内部错误") {
		t.Errorf("响应里应当有一句人话：%s", rec.Body.String())
	}

	// 同一个进程还得能继续服务
	if code, _ := doJSON(t, s, "/api/fs/list", map[string]any{"path": "."}); code != http.StatusOK {
		t.Errorf("panic 之后服务不可用了：%d", code)
	}
}
