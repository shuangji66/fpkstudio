package server

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 文件管理器的整套操作走一遍 HTTP：新建 / 写入 / 读取 / 重命名 / 复制 / 列表 /
// 解压 / 下载 / 删除。这些接口以前一个测试都没有（只有 create 与 upload）。

func fsJSON(t *testing.T, s *Server, path string, body any) (int, map[string]any) {
	t.Helper()
	return doJSON(t, s, path, body)
}

func TestFSOperationsRoundTrip(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)

	if code, _ := fsJSON(t, s, "/api/fs/create", map[string]any{"path": "demo/a.txt"}); code != http.StatusOK {
		t.Fatalf("新建文件状态码 = %d", code)
	}
	if code, _ := fsJSON(t, s, "/api/fs/write", map[string]any{"path": "demo/a.txt", "content": "hello"}); code != http.StatusOK {
		t.Fatalf("写入状态码 = %d", code)
	}
	code, body := fsJSON(t, s, "/api/fs/read", map[string]any{"path": "demo/a.txt"})
	if code != http.StatusOK || body["content"] != "hello" || body["text"] != true {
		t.Fatalf("读取 = %d %v", code, body)
	}
	if _, err := os.Stat(filepath.Join(proj, "a.txt")); err != nil {
		t.Fatalf("文件没落盘: %v", err)
	}

	if code, _ := fsJSON(t, s, "/api/fs/rename", map[string]any{"path": "demo/a.txt", "to": "demo/b.txt"}); code != http.StatusOK {
		t.Fatalf("重命名状态码 = %d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, "a.txt")); err == nil {
		t.Error("旧名字还在")
	}
	if code, _ := fsJSON(t, s, "/api/fs/copy", map[string]any{"path": "demo/b.txt", "to": "demo/c.txt"}); code != http.StatusOK {
		t.Fatalf("复制状态码 = %d", code)
	}

	code, body = fsJSON(t, s, "/api/fs/list", map[string]any{"path": "demo"})
	if code != http.StatusOK {
		t.Fatalf("列目录状态码 = %d", code)
	}
	names := map[string]bool{}
	for _, e := range body["entries"].([]any) {
		names[e.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"b.txt", "c.txt", "manifest"} {
		if !names[want] {
			t.Errorf("列表里缺少 %s（%v）", want, names)
		}
	}

	if code, _ := fsJSON(t, s, "/api/fs/delete", map[string]any{"path": "demo/c.txt"}); code != http.StatusOK {
		t.Fatalf("删除状态码 = %d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, "c.txt")); err == nil {
		t.Error("删除之后文件还在")
	}
}

// 解压：第一次解出文件，第二次同名一律跳过（绝不覆盖用户已有的内容）。
func TestFSExtractSkipsExisting(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range map[string]string{"one.txt": "1", "two.txt": "2"} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	if err := os.WriteFile(filepath.Join(proj, "bundle.tar"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	code, body := fsJSON(t, s, "/api/fs/extract", map[string]any{"path": "demo/bundle.tar"})
	if code != http.StatusOK {
		t.Fatalf("解压状态码 = %d（%v）", code, body)
	}
	if body["files"] != float64(2) || body["skipped"] != float64(0) {
		t.Fatalf("解压结果 = %v", body)
	}

	code, body = fsJSON(t, s, "/api/fs/extract", map[string]any{"path": "demo/bundle.tar"})
	if code != http.StatusOK {
		t.Fatalf("二次解压状态码 = %d", code)
	}
	if body["skipped"] != float64(2) {
		t.Errorf("同名应当跳过 2 个，得到 %v", body)
	}
}

// 下载：单文件回原文 + Content-Length；目录回一个能解析的 zip。
func TestFSDownload(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)
	if err := os.WriteFile(filepath.Join(proj, "b.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/fs/download?path=demo/b.txt", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "hello" {
		t.Fatalf("下载文件 = %d %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "5" {
		t.Errorf("Content-Length = %q", rec.Header().Get("Content-Length"))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/fs/download?path=demo", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("下载目录状态码 = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got == "" {
		t.Error("下载目录应当带 Content-Length（否则用户拿到残包也不知道）")
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("下载回来的不是合法 zip: %v", err)
	}
	names := []string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if !strings.Contains(strings.Join(names, " "), "b.txt") {
		t.Errorf("zip 里应当有 b.txt：%v", names)
	}

	// 下载不存在的东西回 404
	req = httptest.NewRequest(http.MethodGet, "/api/fs/download?path=demo/nope", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("下载不存在 = %d，期望 404", rec.Code)
	}
}

// 路径参数两种形态都要认（查询串 + JSON body），且都要归一成「相对项目根」的写法。
func TestQueryRelNormalization(t *testing.T) {
	s, dataDir := newTestServer(t)
	seedProject(t, dataDir)

	for _, tc := range []struct {
		name  string
		query string
		body  any
		want  string
	}{
		{"根目录（空）", "?path=", nil, "."},
		{"根目录（点）", "?path=.", nil, "."},
		{"越界写法归一成根", "?path=../../", nil, "."},
		{"绝对路径当相对", "?path=/demo", nil, "demo"},
		{"body 里的 path", "", map[string]any{"path": "demo"}, "demo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				raw, _ := json.Marshal(tc.body)
				req = httptest.NewRequest(http.MethodPost, "/api/fs/list", bytes.NewReader(raw))
			} else {
				req = httptest.NewRequest(http.MethodPost, "/api/fs/list"+tc.query, nil)
			}
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("状态码 = %d（%s）", rec.Code, rec.Body.String())
			}
			var out struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Path != tc.want {
				t.Errorf("归一后的 path = %q，期望 %q", out.Path, tc.want)
			}
		})
	}
}

// 解压目标选错（符号链接 / 目录 / 不支持的后缀）都是用户能自己改的，回 400 而不是 500。
func TestFSExtractRejectsBadTarget(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)

	if err := os.WriteFile(filepath.Join(proj, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 项目里指向外部的符号链接：沙箱 API 读不了它，解压也必须拒绝（外部命令吃绝对路径）
	outside := filepath.Join(t.TempDir(), "secret.gz")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(proj, "leak.gz")); err != nil {
		t.Skipf("建不了符号链接: %v", err)
	}

	for _, tc := range []struct {
		name string
		path string
		want int
	}{
		{"符号链接归档", "demo/leak.gz", http.StatusBadRequest},
		{"目录", "demo", http.StatusBadRequest},
		{"不支持的格式", "demo/notes.txt", http.StatusBadRequest},
		{"根目录", ".", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := fsJSON(t, s, "/api/fs/extract", map[string]any{"path": tc.path})
			if code != tc.want {
				t.Errorf("状态码 = %d，期望 %d（%v）", code, tc.want, body)
			}
		})
	}
}
