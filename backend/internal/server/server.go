// Package server 承载 HTTP 层：路由、静态资源、以及所有 API handler。
//
// 只监听 unix socket，不监听任何 TCP 端口；所有路由都挂在可配置的 baseURL 下。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
	"fpkstudio/internal/projects"
	"fpkstudio/internal/state"
	"fpkstudio/internal/trimapi"
)

// Server 是全部依赖的持有者。
type Server struct {
	cfg   *config.Config
	log   *logging.Logger
	fn    *fnpack.Client
	prj   *projects.Manager
	state *state.Store
	trim  *trimapi.Client
	web   fs.FS // 内嵌的前端产物，已经以资源根为根（main 里做过 fs.Sub）

	mux        *http.ServeMux
	startedAt  time.Time
	socketFile os.FileInfo
}

// New 构造 Server。
func New(cfg *config.Config, log *logging.Logger, fn *fnpack.Client, prj *projects.Manager, web fs.FS, st *state.Store, trim *trimapi.Client) *Server {
	s := &Server{
		cfg:       cfg,
		log:       log,
		fn:        fn,
		prj:       prj,
		state:     st,
		trim:      trim,
		mux:       http.NewServeMux(),
		startedAt: time.Now(),
	}
	s.web = web
	s.routes()
	return s
}

// mux 是唯一对外入口。
func (s *Server) Handler() http.Handler { return s.mux }

// ---- 路由 ----

func (s *Server) routes() {
	s.api("/api/system/info", s.handleSystemInfo)
	s.api("/api/system/fnpack", s.handleSystemFnpack)

	s.api("/api/projects", s.handleProjects)
	s.api("/api/projects/create", s.handleProjectCreate)
	s.api("/api/projects/delete", s.handleProjectDelete)
	s.api("/api/projects/exists", s.handleProjectExists)
	s.api("/api/projects/platforms", s.handleProjectPlatforms)
	s.api("/api/projects/settings", s.handleProjectSettings)
	s.api("/api/projects/import", s.handleProjectImport)
	s.api("/api/projects/import-path", s.handleProjectImportPath)
	s.api("/api/projects/pack", s.handleProjectPack)
	s.api("/api/projects/artifacts", s.handleProjectArtifacts)

	s.api("/api/fs/list", s.handleFsList)
	s.api("/api/fs/read", s.handleFsRead)
	s.api("/api/fs/write", s.handleFsWrite)
	s.api("/api/fs/create", s.handleFsCreate)
	s.api("/api/fs/delete", s.handleFsDelete)
	s.api("/api/fs/rename", s.handleFsRename)
	s.api("/api/fs/copy", s.handleFsCopy)
	s.api("/api/fs/upload", s.handleFsUpload)
	s.api("/api/fs/download", s.handleFsDownload)

	s.api("/api/logs", s.handleLogs)
	s.api("/api/logs/stream", s.handleLogStream)

	// 设置（目录 + 飞牛开放 API 的转发）。这些接口一律由服务端代调飞牛接口：
	// 后端 API 只能服务端走 unix socket，token 也不能下发到浏览器。
	s.api("/api/settings", s.handleSettings)
	s.api("/api/trim/status", s.handleTrimStatus)
	s.api("/api/trim/folders", s.handleTrimFolders)
	s.api("/api/trim/folders/delete", s.handleTrimFolderDelete)
	s.api("/api/trim/acl", s.handleTrimACL)
	s.api("/api/trim/semantic", s.handleTrimSemantic)

	// 静态资源兜底，必须是最后一个。
	s.mux.Handle(s.staticPattern(), s.staticHandler())
}

func (s *Server) staticPattern() string {
	if s.cfg.BaseURL == "/" {
		return "/"
	}
	return s.cfg.BaseURL + "/"
}

func (s *Server) api(pattern string, h handlerFunc) {
	full := s.cfg.BaseURL + pattern
	if s.cfg.BaseURL == "/" {
		full = pattern
	}
	s.mux.HandleFunc(full, s.wrap(pattern, h))
}

func (s *Server) wrap(pattern string, h handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		err := h(w, r)
		if err != nil {
			writeError(w, err)
		}
		d := time.Since(start)
		s.log.Debugf("%s %s %s %s", r.Method, pattern, d.Round(time.Millisecond), errText(err))
	}
}

// ---- Run ----

// Run 启动服务并阻塞直到 ctx 结束。
func (s *Server) Run(ctx context.Context) error {
	// 先判断是否已经有实例在跑：unix socket 不会自动清理，不能只凭文件存在与否判断。
	if err := s.ensureNoRunningInstance(); err != nil {
		return err
	}

	ln, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return fmt.Errorf("监听 unix socket 失败: %w", err)
	}
	// 只允许属主和同组成员访问。不鉴权的前提下，这是唯一的防线。
	if err := os.Chmod(s.cfg.SocketPath, 0o660); err != nil {
		s.log.Warnf("设置 socket 权限失败: %v", err)
	}

	// 记下 socket 的 inode：退出时只清理自己的那一个，避免旧实例把新实例的
	// socket 删掉（同名路径被后来者重建时会踩到）。
	if fi, err := os.Stat(s.cfg.SocketPath); err == nil {
		s.socketFile = fi
	}
	if err := s.writePID(); err != nil {
		s.log.Warnf("写 PID 文件失败: %v", err)
	}

	srv := &http.Server{
		Handler:           s.mux,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	s.banner()

	select {
	case err := <-errc:
		s.cleanup()
		return err
	case <-ctx.Done():
	}

	s.log.Infof("收到退出信号，正在停止…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	s.cleanup()
	return nil
}

func (s *Server) banner() {
	addr := s.cfg.SocketPath
	s.log.Infof("fpkstudio %s 已启动", s.cfg.Version)
	s.log.Infof("socket   : unix:%s", addr)
	s.log.Infof("base url : %s", s.cfg.BaseURL)
	s.log.Infof("数据目录 : %s", s.cfg.DataDir)
	s.log.Infof("项目目录 : %s", s.prj.ProjectRoot())
	s.log.Infof("输出目录 : %s", s.prj.OutputDir())
	s.log.Infof("示例     : curl --unix-socket %s %s/api/system/info", addr, s.cfg.BaseURL)
}

func (s *Server) writePID() error {
	return os.WriteFile(s.cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// ensureNoRunningInstance 检查是否已有实例占用 socket，并清掉残留文件。
func (s *Server) ensureNoRunningInstance() error {
	_, err := os.Stat(s.cfg.SocketPath)
	if err != nil {
		return nil // 不存在，直接开
	}

	conn, dialErr := net.DialTimeout("unix", s.cfg.SocketPath, 500*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("已有实例在监听 %s，请先停掉它（或换个 %s）", s.cfg.SocketPath, config.EnvSocket)
	}

	// 连不上，说明是上次异常退出留下的残留文件。
	if err := os.Remove(s.cfg.SocketPath); err != nil {
		return fmt.Errorf("清理残留 socket 失败: %w", err)
	}
	s.log.Infof("清理了残留的 socket 文件 %s", s.cfg.SocketPath)
	return nil
}

func (s *Server) cleanup() {
	if s.socketFile != nil {
		if cur, err := os.Stat(s.cfg.SocketPath); err == nil && os.SameFile(s.socketFile, cur) {
			_ = os.Remove(s.cfg.SocketPath)
		}
	}
	// 只有 PID 文件里还是自己的 pid 才删。
	if data, err := os.ReadFile(s.cfg.PIDFile); err == nil {
		if strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
			_ = os.Remove(s.cfg.PIDFile)
		}
	}
}

// ---- handler 基础设施 ----

// handlerFunc 返回 error，由 wrap 统一转成 HTTP 响应。
type handlerFunc func(http.ResponseWriter, *http.Request) error

// httpError 携带状态码。
type httpError struct {
	Status int
	Msg    string
}

func (e *httpError) Error() string { return e.Msg }

func fail(status int, format string, a ...any) error {
	return &httpError{Status: status, Msg: fmt.Sprintf(format, a...)}
}

func badRequest(format string, a ...any) error { return fail(http.StatusBadRequest, format, a...) }
func notFound(format string, a ...any) error   { return fail(http.StatusNotFound, format, a...) }
func internal(format string, a ...any) error {
	return fail(http.StatusInternalServerError, format, a...)
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var he *httpError
	if errors.As(err, &he) {
		status = he.Status
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return badRequest("请求体为空")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 32<<20))
	if err := dec.Decode(v); err != nil {
		return badRequest("请求体不是合法 JSON: %v", err)
	}
	return nil
}

func errText(err error) string {
	if err == nil {
		return "ok"
	}
	return "ERR: " + err.Error()
}

func queryRel(r *http.Request) (string, error) {
	p, err := pathParam(r)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(p) == "" {
		return ".", nil
	}
	// 路径一律以 projects 根为基准，例如 "myapp/config/privilege"。
	p = strings.TrimPrefix(path.Clean("/"+p), "/")
	return p, nil
}

// pathParam 取出 path 参数：先看查询串，查询串里没有再看 JSON 请求体。
//
// 两种都认，是因为反代 / 网关不保证把查询串原样送到后端；一旦 path 丢了，列目录
// 就退化成列项目根目录或者直接报错，表现为「文件列表永远是空的」。POST + JSON body
// 这条路是稳的——所有写接口本来就走它，所以前端把 list / read 的 path 放进了 body。
// 查询串形式留着，方便 curl 和文档里的例子。
func pathParam(r *http.Request) (string, error) {
	if p := r.URL.Query().Get("path"); p != "" {
		return p, nil
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return "", nil
	}
	if r.Body == nil {
		return "", nil
	}
	var req struct {
		Path string `json:"path"`
	}
	// 解不出来不算错：查询串本来就允许为空（表示项目根目录）。
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	return req.Path, nil
}
