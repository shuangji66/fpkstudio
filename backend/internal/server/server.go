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
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"runtime/debug"
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
	// closing 在关服时关闭：SSE 这类「一直挂着等事件」的 handler 靠它主动退出，
	// 否则 Shutdown 只能等满超时（实测每次 SIGTERM 都要白等 5 秒）。
	closing chan struct{}
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
		closing:   make(chan struct{}),
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
	s.api("/api/projects/artifacts/delete", s.handleProjectArtifactDelete)

	s.api("/api/fs/list", s.handleFsList)
	s.api("/api/fs/read", s.handleFsRead)
	s.api("/api/fs/write", s.handleFsWrite)
	s.api("/api/fs/create", s.handleFsCreate)
	s.api("/api/fs/delete", s.handleFsDelete)
	s.api("/api/fs/rename", s.handleFsRename)
	s.api("/api/fs/copy", s.handleFsCopy)
	s.api("/api/fs/upload", s.handleFsUpload)
	s.api("/api/fs/extract", s.handleFsExtract)
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
		// 一个 handler 里的 panic 不能把整个进程带走（net/http 自己会 recover，但那样
		// 连接直接断掉、日志里也只有一行 stderr）。这里兜住并记进日志器。
		var err error
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					s.log.Errorf("处理 %s 时 panic: %v\n%s", pattern, rec, debug.Stack())
					err = internal("服务内部错误: %v", rec)
				}
			}()
			err = h(w, r)
		}()
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
		// net/http 自己的报错（superfluous WriteHeader、连接被重置…）默认只进 stderr，
		// 而日志弹窗是用户唯一能看到的地方。
		ErrorLog: log.New(logWriter{s.log}, "", 0),
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
	// 先让 SSE 这类长连接 handler 自己退出，再 Shutdown：Shutdown 不会取消请求的
	// context，挂着不动的 handler 只能等满超时。
	close(s.closing)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log.Warnf("优雅停止超时（%v），强制关闭连接", err)
		_ = srv.Close()
	}
	s.cleanup()
	return nil
}

// banner 只留一行「已启动」。
//
// 路径、socket、base url 这些运行参数不再往日志里灌：它们是**每次启动都一样的
// 噪声**，而日志弹窗是给用户看「刚才发生了什么」的地方。要看这些值有两个现成的
// 出口——界面的设置弹窗，以及 `./fpkstudio -paths`。
func (s *Server) banner() {
	s.log.Infof("fpkstudio %s 已启动", s.cfg.Version)
}

func (s *Server) writePID() error {
	if err := os.WriteFile(s.cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return err
	}
	// 飞牛卷上 O_CREAT 的 mode 会被忽略（见 fsx 里的同一处说明），显式补一次：
	// PID 文件读不回来，退出时的清理就少一道判断。
	return os.Chmod(s.cfg.PIDFile, 0o644)
}

// ensureNoRunningInstance 检查是否已有实例占用 socket，并清掉残留文件。
func (s *Server) ensureNoRunningInstance() error {
	fi, err := os.Stat(s.cfg.SocketPath)
	if err != nil {
		return nil // 不存在，直接开
	}

	conn, dialErr := net.DialTimeout("unix", s.cfg.SocketPath, 500*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("已有实例在监听 %s，请先停掉它（或换个 %s）", s.cfg.SocketPath, config.EnvSocket)
	}

	// 连不上，说明是上次异常退出留下的残留。但**只删确实是 socket 的那一个**：
	// 路径填错时（指到一个普通文件上）以前会把那个文件静默删掉。
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s 已存在，但它不是 socket 文件，请检查 %s 是否指错了",
			s.cfg.SocketPath, config.EnvSocket)
	}
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

// maxJSONBody 是请求体的上限。写文件接口也走它，所以给得宽一点。
const maxJSONBody = 32 << 20

// maxUploadBytes 是上传 / 导入单个请求的体积上限（用户要求 1G）。
//
// 以前只给了 ParseMultipartForm 一个「内存阈值」，总量不设限：请求体超出的部分会落到
// 进程的 os.TempDir() 里，一直写下去能把盘灌满。这里用 http.MaxBytesReader 卡住总量。
// 变量而不是常量：测试里把它调小，才能在没有 1G 数据的情况下验「超限回 413」。
var maxUploadBytes int64 = 1 << 30

// uploadMemoryLimit 是 multipart 在内存里保留的字节数，超出部分落临时文件。
const uploadMemoryLimit = 32 << 20

// tooLargeErr 判断错误是不是「请求体超过 MaxBytesReader 的上限」。
func tooLargeErr(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// extractTimeout 是一次解压的上限（外部命令可能被畸形归档拖住）。
const extractTimeout = 10 * time.Minute

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return badRequest("请求体为空")
	}
	// 用 LimitedReader 而不是 io.LimitReader：截断之后要能分辨「超限」和「本来就是
	// 坏 JSON」。以前两者都报「请求体不是合法 JSON: unexpected EOF」，用户存一个
	// 超过 32MB 的文件只会看到这句莫名其妙的话。
	body := &io.LimitedReader{R: r.Body, N: maxJSONBody + 1}
	dec := json.NewDecoder(body)
	if err := dec.Decode(v); err != nil {
		if body.N <= 0 {
			return badRequest("请求体过大（上限 %d MB）", maxJSONBody>>20)
		}
		return badRequest("请求体不是合法 JSON: %v", err)
	}
	if body.N <= 0 {
		return badRequest("请求体过大（上限 %d MB）", maxJSONBody>>20)
	}
	return nil
}

// logWriter 把 net/http 内部的错误转进日志器（见 srv.ErrorLog）。
type logWriter struct{ log *logging.Logger }

func (w logWriter) Write(p []byte) (int, error) {
	if msg := strings.TrimSpace(string(p)); msg != "" {
		w.log.Warnf("http: %s", msg)
	}
	return len(p), nil
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
	if p == "" {
		// 空串是「项目根」：以前 `?path=../../` 会归一成 ""，而前端拿它当目录缓存的键，
		// 于是同一个根目录出现 "" 和 "." 两种写法、缓存对不上。
		p = "."
	}
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
