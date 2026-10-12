package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"fpkstudio/internal/fsx"
	"fpkstudio/internal/projects"
)

// fsError 把文件操作的失败翻成状态码。
//
// 以前这些 handler 一律 notFound / internal，两个方向都不对：
//   - 权限、类型错误也回 404，而 404 在前端是「这个文件还没创建」的正常信号
//     （见 logFsMiss），用户会以为打开的是一个空文件；
//   - 「路径不允许包含 ..」这种用户改一下就能过的错回 500，把真故障淹掉。
func fsError(action string, err error) error {
	switch {
	case errors.Is(err, fsx.ErrNotExist):
		return notFound("%s：%v", action, err)
	case errors.Is(err, fsx.ErrInvalidPath):
		return badRequest("%s：%v", action, err)
	case errors.Is(err, fsx.ErrExists):
		return fail(http.StatusConflict, "%s：%v", action, err)
	case errors.Is(err, fs.ErrPermission):
		return fail(http.StatusForbidden, "%s：%v", action, err)
	default:
		return internal("%s：%v", action, err)
	}
}

// requireMethods 只允许列出的方法。
//
// 以前路由不校验方法：`GET /api/projects/delete` 会走到 decodeJSON，回一句
// 「请求体不是合法 JSON: EOF」——用户完全看不出是自己用错了方法。
func requireMethods(r *http.Request, methods ...string) error {
	for _, m := range methods {
		if r.Method == m {
			return nil
		}
	}
	return fail(http.StatusMethodNotAllowed, "只支持 %s", strings.Join(methods, " / "))
}

func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet, http.MethodPost); err != nil {
		return err
	}
	rel, err := queryRel(r)
	if err != nil {
		return badRequest("%v", err)
	}
	root := s.prj.Root()
	entries, err := root.List(rel)
	if err != nil {
		// 前端拿到失败只会把文件树画成空的，原因必须落在日志里，否则无从查起。
		// 「不存在」是例外：图标目录这类可选目录本来就可能没有，见 logFsMiss。
		s.logFsMiss("列出目录", rel, root.Abs(rel), err)
		return fsError("读取目录失败", err)
	}
	return writeJSON(w, map[string]any{
		"path":    rel,
		"abs":     root.Abs(rel),
		"entries": entries,
	})
}

// logFsMiss 记录一次「读不到」，但**目标不存在时不记**。
//
// 目标不存在在这个应用里是正常状态：向导 / 生命周期脚本常常还没创建（打开它们本来
// 就是「按空内容新建」的入口），图标目录也可能没有。这些都会走 404 返回给前端，
// 前端按「未创建」正常处理，日志里再灌一条警告只会把真正的故障淹掉。
func (s *Server) logFsMiss(action, rel, abs string, err error) {
	if errors.Is(err, fsx.ErrNotExist) {
		return
	}
	s.log.Warnf("%s %s（%s）失败: %v", action, rel, abs, err)
}

type readResponse struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SizeRaw string `json:"sizeRaw"`
	Text    bool   `json:"text"`
	Content string `json:"content"`
	// TooLarge 为真表示「文件太大，没把内容带回来」：编辑器要据此给一句人话，
	// 而不是把一个空文件当成内容显示出来。
	TooLarge bool `json:"tooLarge"`
}

// maxReadBytes 是「可以在编辑器里打开」的文件大小上限。
//
// 以前这个接口把整份文件读进内存再决定要不要回，而 app/server 下几百 MB 的原生程序
// 很常见（上传路径专门做了流式，读路径没有）。超限时只回元信息，让界面提示去下载。
// 变量而不是常量：测试里把它调小，避免真的造一个 8MB 的文件。
var maxReadBytes int64 = 8 << 20

func (s *Server) handleFsRead(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet, http.MethodPost); err != nil {
		return err
	}
	rel, err := queryRel(r)
	if err != nil {
		return badRequest("%v", err)
	}
	root := s.prj.Root()
	st, err := root.Stat(rel)
	if err != nil {
		s.logFsMiss("读取文件", rel, root.Abs(rel), err)
		return fsError("读取文件失败", err)
	}
	if st.Dir {
		return badRequest("目标是目录：%s", rel)
	}

	data, truncated, err := root.ReadFileLimit(rel, maxReadBytes)
	if err != nil {
		s.logFsMiss("读取文件", rel, root.Abs(rel), err)
		return fsError("读取文件失败", err)
	}

	resp := readResponse{Path: rel, Size: st.Size, SizeRaw: fsx.HumanSize(st.Size)}
	if truncated {
		resp.TooLarge = true
		return writeJSON(w, resp)
	}
	// 图标、二进制程序不往编辑器里塞。
	resp.Text = utf8.Valid(data) && bytes.IndexByte(peek(data, 8192), 0) < 0
	if resp.Text {
		resp.Content = string(data)
	}
	return writeJSON(w, resp)
}

func peek(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

type writeReq struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s *Server) handleFsWrite(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req writeReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	if err := s.prj.Root().WriteFile(req.Path, []byte(req.Content), 0o644); err != nil {
		return fsError("写入失败", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.Path, "size": len(req.Content)})
}

type pathReq struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

func (s *Server) handleFsCreate(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	root := s.prj.Root()
	var err error
	if req.Dir {
		// 目录保持 mkdir -p 的语义：重复创建没有破坏性，而「补架构目录」
		// （building/platform/arm|x86）那条路本来就依赖它的幂等性。
		err = root.Mkdir(req.Path, 0o755, true)
	} else {
		// 已存在的文件一律拒绝。WriteFile 走的是 O_CREATE|O_TRUNC，同名时会把用户那份
		// **清成 0 字节**再回一个 ok——「新建文件」时手滑打了个已有文件名，内容就没了，
		// 还没有确认、没有提示。判据与 fsx.Rename 一致（ErrExists）。
		if _, statErr := root.Stat(req.Path); statErr == nil {
			return badRequest("%v：%s（新建文件不会覆盖已有文件）", fsx.ErrExists, req.Path)
		}
		err = root.WriteFile(req.Path, nil, 0o644)
	}
	if err != nil {
		return fsError("创建失败", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.Path})
}

func (s *Server) handleFsDelete(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if err := s.prj.Root().Remove(req.Path, req.Dir); err != nil {
		return fsError("删除失败", err)
	}
	s.log.Infof("删除 %s", req.Path)
	return writeJSON(w, map[string]any{"ok": true})
}

type moveReq struct {
	Path string `json:"path"`
	To   string `json:"to"`
}

func (s *Server) handleFsRename(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req moveReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.To) == "" {
		return badRequest("缺少目标路径")
	}
	if err := s.prj.Root().Rename(req.Path, req.To); err != nil {
		// 「目标已存在」是用户能自己解决的事（换个名字），不该报成服务端错误。
		return fsError("重命名失败", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.To})
}

func (s *Server) handleFsCopy(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req moveReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.To) == "" {
		return badRequest("缺少目标路径")
	}
	if err := s.prj.Root().Copy(req.Path, req.To); err != nil {
		// 「目标已存在」是用户能自己解决的事（换个名字），不报成服务端错误。
		return fsError("复制失败", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.To})
}

// handleFsUpload 支持一次上传多个文件到指定目录。
//
// 上限 maxUploadBytes（1G）：ParseMultipartForm 的参数只是**内存阈值**，超出部分会落到
// 进程的 os.TempDir() 里，总量不设限的话一直传下去就能把盘灌满。这里用 MaxBytesReader
// 卡住整个请求体；客户端取消（关页面 / 点取消）时 ctx 被取消，循环会在文件之间退出，
// 写了一半的那个文件也会被删掉。
func (s *Server) handleFsUpload(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		if tooLargeErr(err) {
			return fail(http.StatusRequestEntityTooLarge, "上传内容超过上限 %s", fsx.HumanSize(maxUploadBytes))
		}
		return badRequest("解析上传失败: %v", err)
	}
	target := r.FormValue("path")
	overwrite := r.FormValue("overwrite") == "true"
	root := s.prj.Root()

	written := make([]map[string]any, 0, 4)
	skipped := 0
	for _, headers := range r.MultipartForm.File {
		for _, hdr := range headers {
			if err := r.Context().Err(); err != nil {
				// 客户端已经走了：别再往下写，已经写好的文件留着（前端会重新拉目录）。
				return fail(http.StatusRequestTimeout, "上传已取消")
			}
			name := path.Base(strings.ReplaceAll(hdr.Filename, "\\", "/"))
			if name == "" || name == "." || name == ".." {
				continue
			}
			dst := name
			if target != "" && target != "." {
				dst = strings.TrimSuffix(target, "/") + "/" + name
			}
			// 回给前端的路径要和真正落盘的位置一致：fsx.Rel 会去掉前导斜杠、
			// 归一 `..`，以前这里原样回 `target + name`，前端的目录缓存键就对不上了。
			if rel, relErr := fsx.Rel(dst); relErr == nil {
				dst = rel
			} else {
				return badRequest("目标路径不合法：%v", relErr)
			}
			// overwrite=false 的语义是「别动已有的文件」。以前这里只省掉了下面那次
			// Remove，而 WriteReader 本身就是 O_TRUNC——开关等于没接上，接口说「跳过」
			// 实际照样覆盖。存在就整份跳过，数量回给调用方。
			if !overwrite {
				if _, statErr := root.Stat(dst); statErr == nil {
					skipped++
					continue
				}
			}
			f, err := hdr.Open()
			if err != nil {
				return internal("读取上传文件失败: %v", err)
			}
			// 边收边写，不把整个文件读进内存：app/server 下放的是原生程序，几百 MB
			// 的二进制在这里很常见。
			size, err := root.WriteReader(dst, ctxReader{ctx: r.Context(), r: f}, s.uploadMode(dst))
			f.Close()
			if err != nil {
				// 写了一半的文件不能留：它顶掉的是同名的那份（overwrite），留在那儿
				// 就是一个看着正常、其实是残渣的文件。
				_ = root.Remove(dst, false)
				if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
					return fail(http.StatusRequestTimeout, "上传已取消")
				}
				return fsError("写入 "+dst+" 失败", err)
			}
			written = append(written, map[string]any{"path": dst, "size": size})
			s.log.Infof("上传 %s（%s）", dst, fsx.HumanSize(size))
		}
	}
	if len(written) == 0 && skipped == 0 {
		return badRequest("没有收到文件")
	}
	return writeJSON(w, map[string]any{"ok": true, "files": written, "skipped": skipped})
}

// handleFsExtract 把压缩包解压到它自己所在的目录。
//
// 支持的格式见 fsx.ArchiveKindOf（tar 全家族 / zip / 单个文件的 gz、bz2、xz、zst），
// 实际解压交给系统的 tar / unzip / gzip 等（见 fsx.Extract）；同名文件一律跳过、
// 不覆盖用户已有的内容，归档里的符号链接也不还原——跳过的数量都回给界面。
func (s *Server) handleFsExtract(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	// 解压走外部命令（tar / unzip / gzip…），给它一个上限：客户端断开时 ctx 会被取消，
	// 子进程跟着被杀；畸形归档（例如几百万个成员的压缩炸弹）也不会把请求挂到天荒地老。
	ctx, cancel := context.WithTimeout(r.Context(), extractTimeout)
	defer cancel()
	res, err := s.prj.Root().Extract(ctx, req.Path)
	if err != nil {
		s.log.Warnf("解压 %s 失败: %v", req.Path, err)
		if errors.Is(err, context.DeadlineExceeded) {
			return internal("解压超时（超过 %s），已中止", extractTimeout)
		}
		return fsError("解压失败", err)
	}
	s.log.Infof(
		"解压 %s 到 %s（%d 个文件，跳过 %d 个已存在，丢弃 %d 个符号链接）",
		req.Path, res.Dir, res.Files, res.Skipped, res.Links,
	)
	return writeJSON(w, res)
}

// handleFsDownload 下载单个文件，或者把目录打成 zip。
//
// 响应头是在开始写正文之前发出的，所以**正文中途出错时没法再回一个 JSON 错误**：
// 那时候再让 wrap 去 writeError，只会在日志里留一条 superfluous WriteHeader、
// 并给浏览器塞一个半截文件。这里改成记日志 + 直接断开——下载失败本来就只能靠
// 浏览器那边发现「文件不完整」。
func (s *Server) handleFsDownload(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet, http.MethodPost); err != nil {
		return err
	}
	rel, err := queryRel(r)
	if err != nil {
		return badRequest("%v", err)
	}
	root := s.prj.Root()
	st, err := root.Stat(rel)
	if err != nil {
		return fsError("目标不存在", err)
	}

	name := path.Base(rel)
	if st.Dir {
		name += ".zip"
		// 先整份打进临时文件，再发正文。
		//
		// 直接往 w 里流式写是不行的：响应头一出去就没法再回一个 JSON 错误，而 ZipDir
		// 中途失败（某个文件读不了——飞牛卷上的 0000 模式文件正是这形态）会留下一个
		// 「结构完整、里面是 0 字节条目」的 zip，用户拿到 200 却永远不知道少了东西。
		// 代价是下载期间多占一份临时空间，换来的是「失败能说清楚」。
		tmp, err := os.CreateTemp(s.cfg.TmpDir, "download-*.zip")
		if err != nil {
			return internal("创建下载临时文件失败: %v", err)
		}
		tmpName := tmp.Name()
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}()

		if err := root.ZipDir(rel, tmp); err != nil {
			s.log.Warnf("打包下载 %s 失败: %v", rel, err)
			return internal("打包目录失败: %v", err)
		}
		size, err := tmp.Seek(0, io.SeekEnd)
		if err != nil {
			return internal("读取下载临时文件失败: %v", err)
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return internal("读取下载临时文件失败: %v", err)
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeader(name)+"\"")
		w.Header().Set("Cache-Control", "no-store")
		if _, err := io.Copy(w, tmp); err != nil {
			s.log.Warnf("下载 %s 中断: %v", rel, err)
		}
		return nil
	}

	f, fi, err := root.OpenFile(rel)
	if err != nil {
		return fsError("下载失败", err)
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeader(name)+"\"")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.Copy(w, f); err != nil {
		s.log.Warnf("下载 %s 中断: %v", rel, err)
	}
	return nil
}

// ctxReader 让「客户端取消」能中断一次正在进行的写入。
//
// io.Copy 读的是它，一旦 ctx 结束（用户关掉页面 / 点了取消）就返回错误，WriteReader
// 随之退出，调用方把写了一半的文件删掉。只靠「文件之间查一次 ctx」是不够的：一个几百
// MB 的文件会一直写到磁盘上。
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// uploadMode 决定上传文件落地时的模式。
//
// 打包时「架构内容落地目录」下面放的是可执行程序（原生应用的二进制直接躺在这里），
// 上传后需要有执行位，否则装完的应用起不来；其余按普通文件处理。
//
// 那个目录是**可配置的**（打包偏好里的 serverDir，默认 app/server）：以前这里写死
// 判 app/server，用户把它改成 app/bin 之后上传的二进制就按 0644 落地，产物里的程序
// 没有执行位，装上去起不来。
func (s *Server) uploadMode(dst string) os.FileMode {
	project, rest, ok := strings.Cut(dst, "/")
	if !ok {
		return 0o644
	}
	serverDir := projects.DefaultServerDir
	if st, err := s.prj.GetSettings(project); err == nil && strings.TrimSpace(st.ServerDir) != "" {
		serverDir = strings.TrimSpace(st.ServerDir)
	}
	serverDir = strings.Trim(serverDir, "/")
	if serverDir == "" {
		return 0o644
	}
	if rest == serverDir || strings.HasPrefix(rest, serverDir+"/") {
		return 0o755
	}
	return 0o644
}

func sanitizeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 32 {
			return '_'
		}
		return r
	}, s)
}
