package server

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"fpkstudio/internal/fsx"
)

func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request) error {
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
		return notFound("读取目录失败: %v", err)
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
}

func (s *Server) handleFsRead(w http.ResponseWriter, r *http.Request) error {
	rel, err := queryRel(r)
	if err != nil {
		return badRequest("%v", err)
	}
	root := s.prj.Root()
	data, err := root.ReadFile(rel)
	if err != nil {
		s.logFsMiss("读取文件", rel, root.Abs(rel), err)
		return notFound("读取文件失败: %v", err)
	}

	resp := readResponse{Path: rel, Size: int64(len(data)), SizeRaw: fsx.HumanSize(int64(len(data)))}
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
	var req writeReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	if err := s.prj.Root().WriteFile(req.Path, []byte(req.Content), 0o644); err != nil {
		return internal("写入失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.Path, "size": len(req.Content)})
}

type pathReq struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
}

func (s *Server) handleFsCreate(w http.ResponseWriter, r *http.Request) error {
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	root := s.prj.Root()
	var err error
	if req.Dir {
		err = root.Mkdir(req.Path, true)
	} else {
		err = root.WriteFile(req.Path, nil, 0o644)
	}
	if err != nil {
		return internal("创建失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.Path})
}

func (s *Server) handleFsDelete(w http.ResponseWriter, r *http.Request) error {
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if err := s.prj.Root().Remove(req.Path, req.Dir); err != nil {
		return internal("删除失败: %v", err)
	}
	s.log.Infof("删除 %s", req.Path)
	return writeJSON(w, map[string]any{"ok": true})
}

type moveReq struct {
	Path string `json:"path"`
	To   string `json:"to"`
}

func (s *Server) handleFsRename(w http.ResponseWriter, r *http.Request) error {
	var req moveReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.To) == "" {
		return badRequest("缺少目标路径")
	}
	if err := s.prj.Root().Rename(req.Path, req.To); err != nil {
		return internal("重命名失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.To})
}

func (s *Server) handleFsCopy(w http.ResponseWriter, r *http.Request) error {
	var req moveReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.To) == "" {
		return badRequest("缺少目标路径")
	}
	if err := s.prj.Root().Copy(req.Path, req.To); err != nil {
		return internal("复制失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "path": req.To})
}

// handleFsUpload 支持一次上传多个文件到指定目录。
func (s *Server) handleFsUpload(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		return badRequest("解析上传失败: %v", err)
	}
	target := r.FormValue("path")
	overwrite := r.FormValue("overwrite") == "true"
	root := s.prj.Root()

	written := make([]map[string]any, 0, 4)
	for _, headers := range r.MultipartForm.File {
		for _, hdr := range headers {
			name := path.Base(strings.ReplaceAll(hdr.Filename, "\\", "/"))
			if name == "" || name == "." || name == ".." {
				continue
			}
			dst := name
			if target != "" && target != "." {
				dst = strings.TrimSuffix(target, "/") + "/" + name
			}
			if overwrite {
				_ = root.Remove(dst, false)
			}
			f, err := hdr.Open()
			if err != nil {
				return internal("读取上传文件失败: %v", err)
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return internal("读取上传文件失败: %v", err)
			}
			if err := root.WriteFile(dst, data, uploadMode(dst)); err != nil {
				return internal("写入 %s 失败: %v", dst, err)
			}
			written = append(written, map[string]any{"path": dst, "size": len(data)})
			s.log.Infof("上传 %s（%s）", dst, fsx.HumanSize(int64(len(data))))
		}
	}
	if len(written) == 0 {
		return badRequest("没有收到文件")
	}
	return writeJSON(w, map[string]any{"ok": true, "files": written})
}

// handleFsExtract 把压缩包解压到它自己所在的目录。
//
// 只认 tar / tar.gz / tgz / gz，实际解压交给系统 tar / gzip（见 fsx.Extract）；
// 同名文件一律跳过，不覆盖用户已有的内容，跳过的数量回给界面。
func (s *Server) handleFsExtract(w http.ResponseWriter, r *http.Request) error {
	var req pathReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	res, err := s.prj.Root().Extract(req.Path)
	if err != nil {
		s.log.Warnf("解压 %s 失败: %v", req.Path, err)
		return internal("解压失败: %v", err)
	}
	s.log.Infof(
		"解压 %s 到 %s（%d 个文件，跳过 %d 个已存在）",
		req.Path, res.Dir, res.Files, res.Skipped,
	)
	return writeJSON(w, res)
}

// handleFsDownload 下载单个文件，或者把目录打成 zip。
func (s *Server) handleFsDownload(w http.ResponseWriter, r *http.Request) error {
	rel, err := queryRel(r)
	if err != nil {
		return badRequest("%v", err)
	}
	root := s.prj.Root()
	st, err := root.Stat(rel)
	if err != nil {
		return notFound("目标不存在: %v", err)
	}

	name := path.Base(rel)
	if st.Dir {
		name += ".zip"
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeader(name)+"\"")
		w.Header().Set("Cache-Control", "no-store")
		return root.ZipDir(rel, w)
	}

	f, fi, err := root.OpenFile(rel)
	if err != nil {
		return notFound("%v", err)
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeader(name)+"\"")
	w.Header().Set("Cache-Control", "no-store")
	_, err = io.Copy(w, f)
	return err
}

// uploadMode 决定上传文件落地时的模式。
//
// app/server/ 下面是应用的可执行程序（原生应用的二进制直接躺在这里），
// 上传后需要有执行位，否则装完的应用起不来；其余按普通文件处理。
func uploadMode(dst string) os.FileMode {
	if strings.Contains(dst, "/app/server/") || strings.HasPrefix(dst, "app/server/") {
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
