package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"fpkstudio/internal/trimapi"
)

// gatewayUser 是统一网关转发的当前使用用户（见飞牛开发文档「统一网关」）。
//
// 网关只在校验过登录态之后才转发 X-Trim-* 这组 Header，所以它们可以作为可信
// 身份用——注意是「可信身份」而不是「已授权」：具体能不能读某个路径，仍然要用
// uid 去问 trim.file.checkUserACL。反过来，请求体 / 查询串里带的用户 ID 一律
// 不认。
//
// 没有网关时（比如用 socat 直接把 socket 桥出来自己调），退化成进程自己的 uid：
// 单用户场景下够用，也能让这套接口在没有 fnOS 的环境里跑通链路。
type gatewayUser struct {
	UID         int    `json:"uid"`
	Name        string `json:"name"`
	IsAdmin     bool   `json:"isAdmin"`
	FromGateway bool   `json:"fromGateway"`
}

func gatewayUserOf(r *http.Request) gatewayUser {
	u := gatewayUser{UID: os.Getuid()}
	raw := strings.TrimSpace(r.Header.Get("X-Trim-Userid"))
	if raw == "" {
		return u
	}
	uid, err := strconv.Atoi(raw)
	if err != nil || uid < 0 {
		return u
	}
	u.UID = uid
	u.Name = strings.TrimSpace(r.Header.Get("X-Trim-Username"))
	u.IsAdmin = strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Trim-Isadmin")), "true")
	u.FromGateway = true
	return u
}

// langOfRequest 取展示语言，给 trim.file.convertPath 用（这个参数是必填的）。
//
// 优先级：请求体（前端的正常路径，见 handleTrimSemantic 的说明）→ 查询串 →
// Accept-Language → zh-CN。
func langOfRequest(r *http.Request, fromBody string) string {
	if lang := strings.TrimSpace(fromBody); lang != "" {
		return lang
	}
	if lang := strings.TrimSpace(r.URL.Query().Get("language")); lang != "" {
		return lang
	}
	if lang := strings.TrimSpace(r.Header.Get("Accept-Language")); lang != "" {
		// 只需要「zh-CN」这种主标签，带权重的列表形式原样传过去反而让系统难判。
		if i := strings.IndexAny(lang, ",;"); i > 0 {
			lang = lang[:i]
		}
		return strings.TrimSpace(lang)
	}
	return "zh-CN"
}

// trimError 把开放 API 的失败翻译成 HTTP 响应。
func trimError(err error) error {
	if err == nil {
		return nil
	}
	if trimapi.IsUnavailable(err) {
		return fail(http.StatusServiceUnavailable, "%v", err)
	}
	var apiErr *trimapi.APIError
	if errors.As(err, &apiErr) {
		// code 1000002 是「管理员权限或 API Scope 不足」，写成明确的一句话，
		// 否则前端只能看到一个数字，排查方向完全没有。
		if apiErr.Code == 1000002 {
			return fail(http.StatusForbidden, "%v（请确认应用包已在 config/resource 声明对应 api-scope）", err)
		}
		return badRequest("%v", err)
	}
	return internal("调用飞牛接口失败: %v", err)
}

// ---- 设置 ----

type dirsPayload struct {
	// ProjectRoot / OutputDir 是当前生效的绝对路径（可能来自用户设置，也可能是默认值）。
	ProjectRoot string `json:"projectRoot"`
	OutputDir   string `json:"outputDir"`
	// 两个 From* 表示该路径是不是用户设置过的——为假时前端显示「默认」。
	ProjectRootFromUser bool `json:"projectRootFromUser"`
	OutputDirFromUser   bool `json:"outputDirFromUser"`
	// 默认值，用于「恢复默认」时给用户看会回到哪里。
	DefaultProjectRoot string `json:"defaultProjectRoot"`
	DefaultOutputDir   string `json:"defaultOutputDir"`
	// OutputEnvIgnored 为真表示 FPKSTUDIO_OUTPUT_DIR 与项目目录撞了、已被忽略，
	// 生效的是默认输出目录。界面要说明这一点，否则用户会以为环境变量没生效。
	OutputEnvIgnored bool `json:"outputEnvIgnored"`
}

type settingsResponse struct {
	Dirs    dirsPayload    `json:"dirs"`
	User    gatewayUser    `json:"user"`
	Trim    trimapi.Status `json:"trim"`
	DataDir string         `json:"dataDir"`
}

func (s *Server) settingsSnapshot(r *http.Request) settingsResponse {
	saved := s.state.Dirs()
	return settingsResponse{
		Dirs: dirsPayload{
			ProjectRoot:         s.prj.ProjectRoot(),
			OutputDir:           s.prj.OutputDir(),
			ProjectRootFromUser: saved.ProjectRoot != "",
			OutputDirFromUser:   saved.OutputDir != "",
			DefaultProjectRoot:  s.cfg.ProjectsDir,
			DefaultOutputDir:    s.cfg.OutputDir,
			OutputEnvIgnored:    s.cfg.OutputEnvIgnored,
		},
		User:    gatewayUserOf(r),
		Trim:    s.trim.Probe(r.Context()),
		DataDir: s.cfg.DataDir,
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) error {
	switch r.Method {
	case http.MethodGet:
		return writeJSON(w, s.settingsSnapshot(r))
	case http.MethodPut, http.MethodPost:
		var req struct {
			ProjectRoot *string `json:"projectRoot"`
			OutputDir   *string `json:"outputDir"`
		}
		if err := decodeJSON(r, &req); err != nil {
			return err
		}

		// 未提到的字段沿用当前保存的值：前端每次只改一个槽位也没问题。
		saved := s.state.Dirs()
		next := saved
		if req.ProjectRoot != nil {
			next.ProjectRoot = strings.TrimSpace(*req.ProjectRoot)
		}
		if req.OutputDir != nil {
			next.OutputDir = strings.TrimSpace(*req.OutputDir)
		}

		// 先真正换一遍（校验 + 打开沙箱），成功了再落盘——反过来的话，一次
		// 打不开的路径会被持久化，下次启动还得靠人去删 state.json。
		if err := s.prj.SetRoots(next.ProjectRoot, next.OutputDir); err != nil {
			return badRequest("%v", err)
		}
		if err := s.state.Save(next); err != nil {
			return internal("保存目录设置失败: %v", err)
		}

		s.log.Infof("目录设置已更新：项目根目录 %s，输出目录 %s", s.prj.ProjectRoot(), s.prj.OutputDir())
		return writeJSON(w, s.settingsSnapshot(r))
	default:
		return fail(http.StatusMethodNotAllowed, "不支持的方法")
	}
}

// ---- 飞牛开放 API 的转发 ----

func (s *Server) handleTrimStatus(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, map[string]any{
		"trim": s.trim.Probe(r.Context()),
		"user": gatewayUserOf(r),
	})
}

// handleTrimFolders 列出当前用户授权给本应用的目录（不含文件授权）。
func (s *Server) handleTrimFolders(w http.ResponseWriter, r *http.Request) error {
	u := gatewayUserOf(r)
	paths, err := s.trim.GetUserAccessibleFolders(r.Context(), u.UID)
	if err != nil {
		return trimError(err)
	}
	return writeJSON(w, map[string]any{"paths": paths})
}

// handleTrimFolderDelete 删除一条目录授权。
//
// 「换目录要删旧授权」由前端串起来：先删旧的，再拉起选择器选新的，
// 否则授权目录会一份份堆在系统里。
func (s *Server) handleTrimFolderDelete(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Path) == "" {
		return badRequest("缺少 path")
	}
	u := gatewayUserOf(r)
	if err := s.trim.DelUserAccessibleFolder(r.Context(), u.UID, req.Path); err != nil {
		return trimError(err)
	}
	s.log.Infof("已删除授权目录 %s（uid=%d）", req.Path, u.UID)
	return writeJSON(w, map[string]any{"ok": true})
}

// handleTrimACL 检查当前用户对若干路径的权限。
func (s *Server) handleTrimACL(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Paths []string `json:"paths"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	u := gatewayUserOf(r)
	list, err := s.trim.CheckUserACL(r.Context(), u.UID, req.Paths)
	if err != nil {
		return trimError(err)
	}
	return writeJSON(w, map[string]any{"list": list})
}

// handleTrimSemantic 把内部路径转成给用户看的语义路径。
//
// language 从**请求体**读，而不是查询串：反代 / 网关不保证把查询串原样送到后端
// （项目里所有接口都因为这件事把参数放进了 body），而 language 是文档要求的必填项，
// 一丢就变成「系统按别的语言返回」甚至转不出来。查询串与 Accept-Language 仍然认，
// 给 curl 用。
func (s *Server) handleTrimSemantic(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Paths    []string `json:"paths"`
		Language string   `json:"language"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	list, err := s.trim.ConvertPath(r.Context(), req.Paths, langOfRequest(r, req.Language))
	if err != nil {
		// 这个接口失败的表现是「界面上退回显示 /vol1/...」，不落日志就只能靠猜。
		s.log.Warnf("转换语义路径 %v 失败: %v", req.Paths, err)
		return trimError(err)
	}
	return writeJSON(w, map[string]any{"list": list})
}

// handleProjectImportPath 导入本机某个路径上的 fpk（飞牛文件选择器选的）。
//
// 路径是前端传上来的，所以这里按当前登录用户再判一次读权限：pickUserFile 只
// 保证「应用用户」拿到了 ACL，不代表当前登录用户能读这个文件。
func (s *Server) handleProjectImportPath(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return badRequest("缺少 path")
	}

	u := gatewayUserOf(r)
	if acls, err := s.trim.CheckUserACL(r.Context(), u.UID, []string{path}); err != nil {
		// 权限服务不可用不该挡住导入本身（应用进程自己有 ACL，读得到）。
		s.log.Warnf("检查 %s 的用户权限失败，跳过校验: %v", path, err)
	} else if len(acls) == 1 && !acls[0].Readable {
		return fail(http.StatusForbidden, "当前用户对 %s 没有读权限", path)
	}

	info, err := s.prj.ImportPath(r.Context(), path)
	if err != nil {
		return internal("%v", err)
	}
	s.log.Infof("从 %s 导入项目 %s", path, info.Name)
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}
