package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"fpkstudio/internal/fsx"
	"fpkstudio/internal/projects"
)

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet); err != nil {
		return err
	}
	list, err := s.prj.List()
	if err != nil {
		return internal("列出项目失败: %v", err)
	}
	return writeJSON(w, map[string]any{"list": list, "count": len(list)})
}

func (s *Server) handleProjectExists(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet); err != nil {
		return err
	}
	info, err := s.prj.Exists(r.URL.Query().Get("name"))
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, info)
}

type createReq struct {
	Name   string `json:"name"`
	Docker bool   `json:"docker"`
	UI     bool   `json:"ui"`
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req createReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return badRequest("项目名不能为空")
	}
	if err := s.prj.Create(r.Context(), req.Name, req.Docker, req.UI); err != nil {
		return projectError(err)
	}
	info, err := s.prj.Info(strings.TrimSpace(req.Name))
	if err != nil {
		// 项目已经建好了，只是信息读不出来（manifest 不完整之类）。照旧回 ok，
		// 但要留一条日志——以前这里是静默的，出问题时无从查起。
		s.log.Warnf("项目 %s 已创建，但读取信息失败: %v", req.Name, err)
		return writeJSON(w, map[string]any{"ok": true})
	}
	s.log.Infof("创建项目 %s（%s）", info.Name, kindOf(info))
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}

func (s *Server) handleProjectDelete(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if err := s.prj.Delete(req.Name); err != nil {
		return projectError(err)
	}
	s.log.Infof("删除项目 %s", req.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleProjectPlatforms(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet); err != nil {
		return err
	}
	list, err := s.prj.Platforms(r.URL.Query().Get("name"))
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]any{"platforms": list})
}

func (s *Server) handleProjectSettings(w http.ResponseWriter, r *http.Request) error {
	name := r.URL.Query().Get("name")
	switch r.Method {
	case http.MethodGet:
		st, err := s.prj.GetSettings(name)
		if err != nil {
			return badRequest("%v", err)
		}
		return writeJSON(w, st)
	case http.MethodPut, http.MethodPost:
		var req struct {
			Name     string            `json:"name"`
			Settings projects.Settings `json:"settings"`
		}
		if err := decodeJSON(r, &req); err != nil {
			return err
		}
		if req.Name == "" {
			req.Name = name
		}
		if err := s.prj.SaveSettings(req.Name, req.Settings); err != nil {
			return internal("%v", err)
		}
		return writeJSON(w, map[string]any{"ok": true})
	default:
		return fail(http.StatusMethodNotAllowed, "不支持的方法")
	}
}

func (s *Server) handleProjectArtifacts(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodGet); err != nil {
		return err
	}
	name := r.URL.Query().Get("name")
	list, err := s.prj.Artifacts(name)
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]any{"list": list, "outputDir": s.prj.OutputDir()})
}

// handleProjectImport 接收上传的 fpk，解包成项目。
func (s *Server) handleProjectImport(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	// 和上传同一条上限（1G）：总量不设限时，超出的部分会落到进程 TMPDIR 里，
	// 而 Import 自己的体积预算管不到「body 还没进 Import」这一段。
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(uploadMemoryLimit); err != nil {
		if tooLargeErr(err) {
			return fail(http.StatusRequestEntityTooLarge, "导入的包超过上限 %s", fsx.HumanSize(maxUploadBytes))
		}
		return badRequest("解析上传失败: %v", err)
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest("缺少 file 字段")
	}
	defer file.Close()

	info, err := s.prj.Import(r.Context(), file, hdr.Filename)
	if err != nil {
		if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
			return fail(http.StatusRequestTimeout, "导入已取消")
		}
		return internal("导入失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}

func (s *Server) handleProjectPack(w http.ResponseWriter, r *http.Request) error {
	if err := requireMethods(r, http.MethodPost); err != nil {
		return err
	}
	var req projects.PackRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return badRequest("缺少项目名")
	}
	results, err := s.prj.Pack(r.Context(), req)
	if err != nil {
		// manifest 字段 / 架构名 / server 目录不合法都是用户能自己改的，回 400。
		return projectError(err)
	}
	ok := 0
	for _, res := range results {
		if res.OK {
			ok++
		}
	}
	return writeJSON(w, map[string]any{"ok": ok == len(results) && ok > 0, "results": results})
}

// projectError 把 projects 包的语义化错误翻译成状态码：项目名不合法 / 已存在 = 用户能
// 自己解决（400），项目不存在 = 404，其余才是服务端故障（500）。以前一律 500，
// 日志里分不清是「用户打错字」还是「真的坏了」。
func projectError(err error) error {
	switch {
	case errors.Is(err, projects.ErrInvalidName), errors.Is(err, projects.ErrExists),
		errors.Is(err, projects.ErrInvalidInput):
		return badRequest("%v", err)
	case errors.Is(err, projects.ErrNotFound):
		return notFound("%v", err)
	default:
		return internal("%v", err)
	}
}

func kindOf(p projects.ProjectInfo) string {
	switch {
	case p.Docker:
		return "docker"
	case p.UI:
		return "native+ui"
	default:
		return "native"
	}
}
