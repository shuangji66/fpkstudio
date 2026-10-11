package server

import (
	"net/http"
	"strings"

	"fpkstudio/internal/projects"
)

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) error {
	list, err := s.prj.List()
	if err != nil {
		return internal("列出项目失败: %v", err)
	}
	return writeJSON(w, map[string]any{"list": list, "count": len(list)})
}

func (s *Server) handleProjectExists(w http.ResponseWriter, r *http.Request) error {
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
	var req createReq
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return badRequest("项目名不能为空")
	}
	if err := s.prj.Create(r.Context(), req.Name, req.Docker, req.UI); err != nil {
		return internal("%v", err)
	}
	info, err := s.prj.Info(strings.TrimSpace(req.Name))
	if err != nil {
		return writeJSON(w, map[string]any{"ok": true})
	}
	s.log.Infof("创建项目 %s（%s）", info.Name, kindOf(info))
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}

func (s *Server) handleProjectDelete(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if err := s.prj.Delete(req.Name); err != nil {
		return internal("%v", err)
	}
	s.log.Infof("删除项目 %s", req.Name)
	return writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleProjectPlatforms(w http.ResponseWriter, r *http.Request) error {
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
	name := r.URL.Query().Get("name")
	list, err := s.prj.Artifacts(name)
	if err != nil {
		return badRequest("%v", err)
	}
	return writeJSON(w, map[string]any{"list": list, "outputDir": s.cfg.OutputDir})
}

// handleProjectImport 接收上传的 fpk，解包成项目。
func (s *Server) handleProjectImport(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return badRequest("解析上传失败: %v", err)
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest("缺少 file 字段")
	}
	defer file.Close()

	info, err := s.prj.Import(r.Context(), file, hdr.Filename)
	if err != nil {
		return internal("导入失败: %v", err)
	}
	return writeJSON(w, map[string]any{"ok": true, "project": info})
}

func (s *Server) handleProjectPack(w http.ResponseWriter, r *http.Request) error {
	var req projects.PackRequest
	if err := decodeJSON(r, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Name) == "" {
		return badRequest("缺少项目名")
	}
	results, err := s.prj.Pack(r.Context(), req)
	if err != nil {
		return internal("%v", err)
	}
	ok := 0
	for _, res := range results {
		if res.OK {
			ok++
		}
	}
	return writeJSON(w, map[string]any{"ok": ok == len(results) && ok > 0, "results": results})
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
