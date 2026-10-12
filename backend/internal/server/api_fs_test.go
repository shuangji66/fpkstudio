package server

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
	"fpkstudio/internal/projects"
	"fpkstudio/internal/state"
	"fpkstudio/internal/trimapi"
)

// newTestServer 造一个挂在临时数据目录上的 Server（不嵌前端，只测 API）。
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	return newTestServerWeb(t, "/", os.DirFS(t.TempDir()))
}

// newTestServerWeb 同上，额外指定 baseURL 与内嵌的前端产物（测静态资源用）。
func newTestServerWeb(t *testing.T, baseURL string, web fs.FS) (*Server, string) {
	t.Helper()

	dataDir := t.TempDir()
	cfg := &config.Config{
		Version:     "test",
		DataDir:     dataDir,
		ProjectsDir: filepath.Join(dataDir, "projects"),
		OutputDir:   filepath.Join(dataDir, "output"),
		BaseURL:     baseURL,
		FnpackBin:   "fnpack",
		LogLevel:    "error",
		StateFile:   filepath.Join(dataDir, "state.json"),
	}
	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	fn := fnpack.New(cfg, log)
	prj, err := projects.New(cfg, log, fn)
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}
	t.Cleanup(prj.Close)

	s := New(cfg, log, fn, prj, web, state.Empty(cfg.StateFile), trimapi.New("", "", log))
	return s, dataDir
}

// doJSON 发一个 JSON 请求，返回状态码与响应体。
func doJSON(t *testing.T, s *Server, path string, body any) (int, map[string]any) {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("序列化请求体失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec.Code, out
}

// 造一个最小项目：只有 manifest。
func seedProject(t *testing.T, dataDir string) string {
	t.Helper()

	proj := filepath.Join(dataDir, "projects", "demo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("建项目目录失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, "manifest"), []byte("appname = demo\n"), 0o644); err != nil {
		t.Fatalf("写 manifest 失败: %v", err)
	}
	return proj
}

// 「新建文件」打到已有文件名上时，以前会把它清成 0 字节（fsx.WriteFile 是 O_TRUNC）
// 还回一个 ok；现在必须拒绝，且内容一点都不能动。
func TestFSCreateRefusesExistingFile(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)

	status, body := doJSON(t, s, "/api/fs/create", map[string]any{"path": "demo/manifest"})
	if status != http.StatusBadRequest {
		t.Fatalf("同名新建：状态码 = %d（%v），期望 400", status, body)
	}
	data, err := os.ReadFile(filepath.Join(proj, "manifest"))
	if err != nil {
		t.Fatalf("读 manifest 失败: %v", err)
	}
	if string(data) != "appname = demo\n" {
		t.Fatalf("已有文件被改动了：%q", data)
	}

	// 新文件照常创建
	if status, body = doJSON(t, s, "/api/fs/create", map[string]any{"path": "demo/new.txt"}); status != http.StatusOK {
		t.Fatalf("新建文件：状态码 = %d（%v），期望 200", status, body)
	}
	if _, err := os.Stat(filepath.Join(proj, "new.txt")); err != nil {
		t.Fatalf("新文件没有落地: %v", err)
	}

	// 目录保持幂等（「补架构目录」那条路依赖它）
	if status, body = doJSON(t, s, "/api/fs/create", map[string]any{"path": "demo/building/platform/arm", "dir": true}); status != http.StatusOK {
		t.Fatalf("建目录：状态码 = %d（%v），期望 200", status, body)
	}
	if status, body = doJSON(t, s, "/api/fs/create", map[string]any{"path": "demo/building/platform/arm", "dir": true}); status != http.StatusOK {
		t.Fatalf("重复建目录：状态码 = %d（%v），期望 200", status, body)
	}

	// 空 path 不该去动根目录
	if status, _ = doJSON(t, s, "/api/fs/create", map[string]any{"path": "  "}); status != http.StatusBadRequest {
		t.Fatalf("空 path：状态码 = %d，期望 400", status)
	}
}

// 目录设置是应用级的：改完之后这个应用就能读写那个目录，所以经过网关时必须管理员才能改。
// 没经过网关（直接调 socket / 开发）时保持原行为——那种部署下 socket 权限就是边界。
func TestSettingsRequiresAdminBehindGateway(t *testing.T) {
	s, dataDir := newTestServer(t)
	root := filepath.Join(t.TempDir(), "newroot")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	defaultRoot := filepath.Join(dataDir, "projects")

	put := func(withGateway bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"projectRoot": root})
		req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if withGateway {
			req.Header.Set("X-Trim-Userid", "1000")
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	if status, body := put(true); status != http.StatusForbidden {
		t.Fatalf("网关下的普通用户：状态码 = %d（%v），期望 403", status, body)
	}
	if got := s.prj.ProjectRoot(); got != defaultRoot {
		t.Fatalf("被拒绝之后目录不该变：%q", got)
	}

	// 没有网关身份：照旧可用（本地开发 / 单用户部署）
	if status, body := put(false); status != http.StatusOK {
		t.Fatalf("无网关时应当可用：状态码 = %d（%v）", status, body)
	}
	if got := s.prj.ProjectRoot(); got != root {
		t.Fatalf("目录没换过去：%q", got)
	}
}

// 上传的 overwrite 语义：false = 已存在的文件跳过（一个字节都不动），true = 覆盖。
func TestFSUploadOverwriteSemantics(t *testing.T) {
	s, dataDir := newTestServer(t)
	proj := seedProject(t, dataDir)

	upload := func(name, content string, overwrite bool) (int, map[string]any) {
		t.Helper()

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.WriteField("path", "demo"); err != nil {
			t.Fatalf("写 path 字段失败: %v", err)
		}
		flag := "false"
		if overwrite {
			flag = "true"
		}
		if err := w.WriteField("overwrite", flag); err != nil {
			t.Fatalf("写 overwrite 字段失败: %v", err)
		}
		part, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatalf("建表单文件失败: %v", err)
		}
		if _, err := io.WriteString(part, content); err != nil {
			t.Fatalf("写表单内容失败: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("关闭 multipart 失败: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/fs/upload", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	// 第一次上传：没有同名文件，照常写入
	if status, body := upload("keep.txt", "FIRST\n", false); status != http.StatusOK {
		t.Fatalf("首次上传：状态码 = %d（%v）", status, body)
	}
	// overwrite=false：跳过，内容不动
	status, body := upload("keep.txt", "SECOND\n", false)
	if status != http.StatusOK {
		t.Fatalf("跳过上传：状态码 = %d（%v）", status, body)
	}
	data, err := os.ReadFile(filepath.Join(proj, "keep.txt"))
	if err != nil {
		t.Fatalf("读文件失败: %v", err)
	}
	if string(data) != "FIRST\n" {
		t.Fatalf("overwrite=false 时被覆盖了：%q", data)
	}
	if skipped, _ := body["skipped"].(float64); skipped != 1 {
		t.Fatalf("跳过数 = %v，期望 1（%v）", body["skipped"], body)
	}
	// overwrite=true：覆盖
	if status, body = upload("keep.txt", "THIRD\n", true); status != http.StatusOK {
		t.Fatalf("覆盖上传：状态码 = %d（%v）", status, body)
	}
	if data, _ = os.ReadFile(filepath.Join(proj, "keep.txt")); string(data) != "THIRD\n" {
		t.Fatalf("overwrite=true 没有覆盖：%q", data)
	}
}
