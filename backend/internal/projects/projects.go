package projects

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/fsx"
	"fpkstudio/internal/logging"
)

const (
	// fnpack 默认从 app/server 取原生程序。
	defaultServerDir = "app/server"
	// 打包偏好存放位置（放在项目内的 building/，跟 fnpackup 保持一致）。
	settingsFile = "building/settings.json"
	// 导入 fpk 时的体积上限，防解压炸弹。
	maxImportBytes = 8 << 30
	maxImportFiles = 200000
)

// ProjectInfo 是项目列表里的一项。
type ProjectInfo struct {
	Name        string `json:"name"`
	AppName     string `json:"appname"`
	Version     string `json:"version"`
	DisplayName string `json:"displayName"`
	Platform    string `json:"platform"`
	Docker      bool   `json:"docker"`
	UI          bool   `json:"ui"`
	Building    bool   `json:"building"`
	ModTime     string `json:"modTime"`
}

// ExistsInfo 描述一个项目包含哪些形态。
type ExistsInfo struct {
	Exists bool `json:"exists"`
	Docker bool `json:"docker"`
	UI     bool `json:"ui"`
}

// Settings 是打包偏好，持久化在项目内。
type Settings struct {
	// MultiPlatform 为真时按 Platforms 逐个架构打包。
	MultiPlatform bool `json:"multiPlatform"`
	// Platforms 是选中的架构目录名（对应 building/platform/<name>）。
	Platforms []string `json:"platforms"`
	// ServerDir 是架构内容的落地目录，相对项目根。
	ServerDir string `json:"serverDir"`
	// CopyToOutput 为真时额外把产物复制到 FPKSTUDIO_OUTPUT_DIR。
	CopyToOutput bool `json:"copyToOutput"`
}

// PackRequest 是一次打包请求。
type PackRequest struct {
	Name      string   `json:"name"`
	Platforms []string `json:"platforms"`
	ServerDir string   `json:"serverDir"`
	CopyOut   bool     `json:"copyOut"`
}

// PackResult 是单个架构的打包结果。
type PackResult struct {
	Platform string `json:"platform"`
	OK       bool   `json:"ok"`
	File     string `json:"file"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Log      string `json:"log"`
}

// Manager 是项目层的全部业务逻辑。
type Manager struct {
	cfg    *config.Config
	log    *logging.Logger
	fn     *fnpack.Client
	home   *fsx.Root // 数据根目录，用于暂存
	prj    *fsx.Root // projects 目录
	output *fsx.Root // 打包输出目录（可能在数据根之外）

	// prjInHome 是 projects 目录相对于数据根目录的位置，
	// 用于在同一个 home 沙箱里跨目录复制。
	prjInHome string
}

// New 创建 Manager。
func New(cfg *config.Config, log *logging.Logger, fn *fnpack.Client) (*Manager, error) {
	home, err := fsx.Open(cfg.DataDir, true)
	if err != nil {
		return nil, fmt.Errorf("打开数据目录失败: %w", err)
	}
	prj, err := fsx.Open(cfg.ProjectsDir, true)
	if err != nil {
		home.Close()
		return nil, fmt.Errorf("打开项目目录失败: %w", err)
	}
	out, err := fsx.Open(cfg.OutputDir, true)
	if err != nil {
		prj.Close()
		home.Close()
		return nil, fmt.Errorf("打开输出目录失败: %w", err)
	}

	rel, err := filepath.Rel(cfg.DataDir, cfg.ProjectsDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		prj.Close()
		home.Close()
		out.Close()
		return nil, fmt.Errorf("项目目录必须位于数据目录内：%s", cfg.ProjectsDir)
	}

	return &Manager{
		cfg:       cfg,
		log:       log,
		fn:        fn,
		home:      home,
		prj:       prj,
		output:    out,
		prjInHome: filepath.ToSlash(rel),
	}, nil
}

// Close 释放句柄。
func (m *Manager) Close() {
	for _, r := range []*fsx.Root{m.prj, m.home, m.output} {
		if r != nil {
			_ = r.Close()
		}
	}
}

// Root 暴露项目目录沙箱，供文件管理器复用。
func (m *Manager) Root() *fsx.Root { return m.prj }

// List 列出全部项目。
func (m *Manager) List() ([]ProjectInfo, error) {
	entries, err := m.prj.List(".")
	if err != nil {
		return nil, err
	}
	out := make([]ProjectInfo, 0, len(entries))
	for _, e := range entries {
		if !e.Dir {
			continue
		}
		info, err := m.Info(e.Name)
		if err != nil {
			// 单个项目目录损坏不应该让整个列表挂掉。
			out = append(out, ProjectInfo{Name: e.Name, ModTime: e.ModTime})
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

// Info 读取单个项目的信息。
func (m *Manager) Info(name string) (ProjectInfo, error) {
	rel, err := projectPath(name)
	if err != nil {
		return ProjectInfo{}, err
	}
	st, err := m.prj.Stat(rel)
	if err != nil {
		return ProjectInfo{}, err
	}
	if !st.Dir {
		return ProjectInfo{}, fmt.Errorf("%s 不是目录", name)
	}

	info := ProjectInfo{Name: name, ModTime: st.ModTime}
	if data, err := m.prj.ReadFile(rel + "/manifest"); err == nil {
		kv := parseManifest(string(data))
		info.AppName = kv["appname"]
		info.Version = kv["version"]
		info.DisplayName = kv["display_name"]
		info.Platform = kv["platform"]
	}
	info.Docker = m.exists(rel + "/app/docker/docker-compose.yaml")
	info.UI = m.exists(rel + "/app/ui/config")
	info.Building = m.exists(rel + "/building")
	return info, nil
}

// Exists 返回项目的形态信息。
func (m *Manager) Exists(name string) (ExistsInfo, error) {
	rel, err := projectPath(name)
	if err != nil {
		return ExistsInfo{}, err
	}
	st, err := m.prj.Stat(rel)
	if err != nil {
		return ExistsInfo{Exists: false}, nil
	}
	return ExistsInfo{
		Exists: st.Dir,
		Docker: m.exists(rel + "/app/docker/docker-compose.yaml"),
		UI:     m.exists(rel + "/app/ui/config"),
	}, nil
}

// Create 调用 fnpack 创建项目骨架，并补齐 fpkstudio 需要的工作目录。
func (m *Manager) Create(ctx context.Context, name string, docker, withUI bool) error {
	rel, err := projectPath(name)
	if err != nil {
		return err
	}
	if _, err := m.prj.Stat(rel); err == nil {
		return fmt.Errorf("项目 %s 已存在", name)
	}

	if _, err := m.fn.Create(ctx, m.cfg.ProjectsDir, name, docker, withUI); err != nil {
		return fmt.Errorf("fnpack create 失败: %w", err)
	}
	if _, err := m.prj.Stat(rel); err != nil {
		return fmt.Errorf("fnpack 报告成功，但目录 %s 没有生成", rel)
	}

	// fnpack 模板里带了 .DS_Store，在部分文件系统上会让后续 build 直接失败。
	if n := stripJunk(m.prj, rel); n > 0 {
		m.log.Infof("清理模板里的冗余文件 %d 个", n)
	}
	for _, d := range []string{rel + "/building", rel + "/building/platform"} {
		if err := m.prj.Mkdir(d, true); err != nil {
			return err
		}
	}
	return nil
}

// Delete 删除项目目录。
func (m *Manager) Delete(name string) error {
	rel, err := projectPath(name)
	if err != nil {
		return err
	}
	if _, err := m.prj.Stat(rel); err != nil {
		return fmt.Errorf("项目 %s 不存在", name)
	}
	return m.prj.Remove(rel, true)
}

// Platforms 列出 building/platform 下的架构目录。
func (m *Manager) Platforms(name string) ([]string, error) {
	rel, err := projectPath(name)
	if err != nil {
		return nil, err
	}
	entries, err := m.prj.List(rel + "/building/platform")
	if err != nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Dir {
			out = append(out, e.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Artifact 是一个打包产物。
type Artifact struct {
	Name    string `json:"name"`
	Scope   string `json:"scope"` // project | output
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SizeRaw string `json:"sizeRaw"`
	ModTime string `json:"modTime"`
}

// Artifacts 列出与该项目相关的 fpk：项目目录里的，以及输出目录里同名的。
func (m *Manager) Artifacts(name string) ([]Artifact, error) {
	rel, err := projectPath(name)
	if err != nil {
		return nil, err
	}
	appname := name
	if data, err := m.prj.ReadFile(rel + "/manifest"); err == nil {
		if a := strings.TrimSpace(parseManifest(string(data))["appname"]); a != "" {
			appname = a
		}
	}

	out := make([]Artifact, 0, 4)
	collect := func(root *fsx.Root, dirRel, scope string, filter func(string) bool) {
		entries, err := root.List(dirRel)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.Dir || !strings.HasSuffix(strings.ToLower(e.Name), ".fpk") {
				continue
			}
			if filter != nil && !filter(e.Name) {
				continue
			}
			p := e.Name
			if dirRel != "." {
				p = dirRel + "/" + e.Name
			}
			out = append(out, Artifact{
				Name:    e.Name,
				Scope:   scope,
				Path:    root.Abs(p),
				Size:    e.Size,
				SizeRaw: fsx.HumanSize(e.Size),
				ModTime: e.ModTime,
			})
		}
	}
	collect(m.prj, rel, "project", nil)
	collect(m.output, ".", "output", func(n string) bool {
		return strings.HasPrefix(n, appname+"-") || n == appname+".fpk"
	})

	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// GetSettings 读取打包偏好，文件不存在时返回默认值。
func (m *Manager) GetSettings(name string) (Settings, error) {
	rel, err := projectPath(name)
	if err != nil {
		return Settings{}, err
	}
	s := Settings{ServerDir: defaultServerDir, CopyToOutput: true}
	data, err := m.prj.ReadFile(rel + "/" + settingsFile)
	if err != nil {
		return s, nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, nil
	}
	if s.ServerDir == "" {
		s.ServerDir = defaultServerDir
	}
	return s, nil
}

// SaveSettings 写入打包偏好。
func (m *Manager) SaveSettings(name string, s Settings) error {
	rel, err := projectPath(name)
	if err != nil {
		return err
	}
	if s.ServerDir == "" {
		s.ServerDir = defaultServerDir
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return m.prj.WriteFile(rel+"/"+settingsFile, data, 0o644)
}

// Pack 执行打包。
//
// 关键设计：**不改动用户的项目文件**。fnpackup 是在原地改 manifest.platform
// 并覆盖 app/server，打包一次就把项目写脏了；这里改为把项目整体复制到
// 数据目录下的暂存区，在副本上改、在副本上 build，结束后暂存区整个删掉。
func (m *Manager) Pack(ctx context.Context, req PackRequest) ([]PackResult, error) {
	rel, err := projectPath(req.Name)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := m.prj.ReadFile(rel + "/manifest")
	if err != nil {
		return nil, fmt.Errorf("读取 manifest 失败: %w", err)
	}
	kv := parseManifest(string(manifestBytes))
	appname := strings.TrimSpace(kv["appname"])
	if appname == "" {
		return nil, errors.New("manifest 里没有 appname")
	}
	version := strings.TrimSpace(kv["version"])
	if version == "" {
		version = "0.0.0"
	}
	if kv["platform"] == "" {
		return nil, errors.New("manifest 里没有 platform（旧版可能叫 arch，请改名为 platform）")
	}

	serverDir := strings.TrimSpace(req.ServerDir)
	if serverDir == "" {
		serverDir = defaultServerDir
	}
	serverRel, err := fsx.Rel(serverDir)
	if err != nil {
		return nil, fmt.Errorf("server 目录非法: %w", err)
	}

	targets := make([]string, 0, len(req.Platforms))
	for _, p := range req.Platforms {
		if p = strings.TrimSpace(p); p != "" {
			targets = append(targets, p)
		}
	}
	single := len(targets) == 0

	stageName := fmt.Sprintf("build-%s-%d", sanitize(req.Name), time.Now().UnixNano())
	stage := "tmp/" + stageName
	if err := m.home.Mkdir(stage, true); err != nil {
		return nil, fmt.Errorf("创建暂存目录失败: %w", err)
	}
	defer func() {
		if err := m.home.Remove(stage, true); err != nil {
			m.log.Warnf("清理暂存目录失败: %v", err)
		}
	}()

	if err := m.home.Copy(m.prjInHome+"/"+rel, stage+"/project"); err != nil {
		return nil, fmt.Errorf("复制项目到暂存区失败: %w", err)
	}
	// 修复不可用的权限（见 fsx.RepairModes 的说明），保证 fnpack 读得到、
	// 且产物里的模式位可用。
	if n, err := m.home.RepairModes(stage+"/project", []string{"cmd"}); err != nil {
		m.log.Warnf("修正暂存区权限时出错: %v", err)
	} else if n > 0 {
		m.log.Infof("修正了 %d 个文件/目录的权限", n)
	}
	if os.Getenv("FPKSTUDIO_DEBUG_MODES") != "" {
		m.dumpModes(stage)
	}
	stagePrj := m.home.Abs(stage + "/project")

	m.log.Infof("开始打包 %s (%s)，输出目录 %s", req.Name, version, m.cfg.OutputDir)

	rounds := targets
	if single {
		rounds = []string{""} // 单包：沿用 manifest 里已填的 platform
	}

	results := make([]PackResult, 0, len(rounds))
	for _, platform := range rounds {
		res := PackResult{Platform: platform}
		if platform == "" {
			res.Platform = strings.TrimSpace(kv["platform"])
		}

		if platform != "" {
			if err := m.prepareStage(stage, rel, serverRel, platform); err != nil {
				res.OK = false
				res.Log = err.Error()
				m.log.Errorf("准备 %s 的 %s 内容失败: %v", req.Name, platform, err)
				results = append(results, res)
				continue
			}
			if err := setManifestPlatform(filepath.Join(stagePrj, "manifest"), platform); err != nil {
				res.OK = false
				res.Log = err.Error()
				results = append(results, res)
				continue
			}
		}

		if os.Getenv("FPKSTUDIO_DEBUG_MODES") != "" {
			m.dumpModes(stage)
		}
		out, buildErr := m.fn.Build(ctx, stagePrj)
		res.Log = tail(out, 4000)

		src := filepath.Join(stagePrj, appname+".fpk")
		if buildErr != nil {
			res.OK = false
			if _, statErr := os.Stat(src); statErr != nil {
				if res.Log == "" {
					res.Log = buildErr.Error()
				}
				results = append(results, res)
				continue
			}
			m.log.Warnf("fnpack 退出码非零，但产物已生成，继续处理：%v", buildErr)
		}

		st, statErr := os.Stat(src)
		if statErr != nil {
			res.OK = false
			if res.Log == "" {
				res.Log = fmt.Sprintf("未找到打包产物 %s", filepath.Base(src))
			}
			results = append(results, res)
			continue
		}

		fileName := fmt.Sprintf("%s-%s-%s.fpk", appname, version, res.Platform)
		inProject := m.prj.Abs(rel + "/" + fileName)
		if err := copyFile(src, inProject); err != nil {
			res.OK = false
			res.Log = fmt.Sprintf("写入项目目录失败: %v", err)
			results = append(results, res)
			continue
		}
		res.File = fileName
		res.Path = inProject
		res.Size = st.Size()

		if req.CopyOut {
			dst := m.output.Abs(fileName)
			if err := copyFile(src, dst); err != nil {
				res.OK = false
				res.Log = fmt.Sprintf("复制到输出目录失败: %v", err)
				results = append(results, res)
				continue
			}
			res.Path = dst
		}

		res.OK = true
		m.log.Infof("打包成功 %s（%s）", fileName, fsx.HumanSize(st.Size()))
		results = append(results, res)
	}
	return results, nil
}

// prepareStage 把 building/platform/<platform>/ 的内容覆盖到暂存区的 server 目录。
//
// 约定与 fnpackup 一致：架构目录里放的是「该架构下 app/<serverDir> 应有的全部内容」。
func (m *Manager) prepareStage(stage string, prjRel string, serverRel string, platform string) error {
	srcInPrj := prjRel + "/building/platform/" + platform
	srcInHome := m.prjInHome + "/" + srcInPrj

	srcStat, err := m.prj.Stat(srcInPrj)
	if err != nil {
		return fmt.Errorf("找不到架构目录 building/platform/%s", platform)
	}
	if !srcStat.Dir {
		return fmt.Errorf("building/platform/%s 不是目录", platform)
	}
	children, err := m.prj.List(srcInPrj)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		m.log.Warnf("架构目录 building/platform/%s 是空的，将以空 server 目录打包", platform)
	}

	dstRel := stage + "/project/" + serverRel
	// 目标可能不存在（fnpack 只创建 app/server 与 app/www），不存在的错误直接忽略。
	_ = m.home.Remove(dstRel, true)
	if err := m.home.Mkdir(dstRel, true); err != nil {
		return err
	}
	return m.home.Copy(srcInHome, dstRel)
}

// Import 把上传的 .fpk 解成一个项目目录。
func (m *Manager) Import(ctx context.Context, r io.Reader, fileName string) (ProjectInfo, error) {
	stage := fmt.Sprintf("tmp/import-%d", time.Now().UnixNano())
	if err := m.home.Mkdir(stage, true); err != nil {
		return ProjectInfo{}, err
	}
	defer func() { _ = m.home.Remove(stage, true) }()

	if err := unpackFPK(m.home, stage, r); err != nil {
		return ProjectInfo{}, err
	}

	// fnpack 把 config/ 同时放进了 app.tgz，解出来会在 app/ 下产生一份重复目录，
	// 这里删掉它，只保留根目录下的 config/。
	if _, err := m.home.Stat(stage + "/app/config"); err == nil {
		_ = m.home.Remove(stage+"/app/config", true)
	}

	m.stripJunkHome(stage)

	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if data, err := m.home.ReadFile(stage + "/manifest"); err == nil {
		if app := strings.TrimSpace(parseManifest(string(data))["appname"]); app != "" {
			name = app
		}
	}
	name = sanitize(name)
	if name == "" {
		name = fmt.Sprintf("imported-%d", time.Now().Unix())
	}

	rel, err := projectPath(name)
	if err != nil {
		return ProjectInfo{}, err
	}
	if _, err := m.prj.Stat(rel); err == nil {
		name = fmt.Sprintf("%s-%d", name, time.Now().Unix())
		rel, err = projectPath(name)
		if err != nil {
			return ProjectInfo{}, err
		}
	}

	if err := os.Rename(m.home.Abs(stage), m.prj.Abs(rel)); err != nil {
		return ProjectInfo{}, fmt.Errorf("移动到项目目录失败: %w", err)
	}
	_ = m.prj.Mkdir(rel+"/building/platform", true)

	m.log.Infof("导入项目 %s 完成", name)
	return m.Info(name)
}

// ---- 内部工具 ----

func (m *Manager) exists(rel string) bool {
	_, err := m.prj.Stat(rel)
	return err == nil
}

// stripJunk 删除 macOS / Windows 的元数据文件，返回删除数量。
func stripJunk(root *fsx.Root, rel string) int {
	n := 0
	_ = root.Walk(rel, func(cur string, isDir bool, name string) error {
		if isDir {
			return nil
		}
		switch name {
		case ".DS_Store", "Thumbs.db", "desktop.ini":
			if err := root.Remove(cur, false); err == nil {
				n++
			}
		}
		return nil
	})
	return n
}

// stripJunkHome 同上，但作用于数据根目录下的路径（导入时的暂存区）。
func (m *Manager) stripJunkHome(rel string) { stripJunk(m.home, rel) }

// dumpModes 在 FPKSTUDIO_DEBUG_MODES 打开时打印暂存区的权限，用于排查
// 「产物里权限不对」这类问题。
func (m *Manager) dumpModes(stage string) {
	_ = m.home.Walk(stage+"/project", func(cur string, isDir bool, _ string) error {
		if fi, err := os.Lstat(m.home.Abs(cur)); err == nil {
			m.log.Debugf("mode %04o %s", fi.Mode().Perm(), strings.TrimPrefix(cur, stage+"/project/"))
		}
		return nil
	})
}

func projectPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("项目名不能为空")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", errors.New("项目名不能包含路径分隔符")
	}
	return fsx.Rel(name)
}

func sanitize(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// setManifestPlatform 在暂存区的 manifest 里改 platform，保留其余字节。
func setManifestPlatform(manifestPath, platform string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	lines := splitLines(string(data))
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		if strings.TrimSpace(trimmed[:eq]) != "platform" {
			continue
		}
		lines[i] = "platform=" + platform
		found = true
		break
	}
	if !found {
		lines = append(lines[:len(lines)-1], "platform="+platform, "")
	}
	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// 同上：创建时的模式位不一定被文件系统接受。
	return out.Chmod(0o644)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// unpackFPK 解开 fpk。
//
// 包结构（用真机 fnpack 1.2.4 验证过）：
//
//	fpk (gzip tar)
//	├── app.tgz            -> app/ 下的内容，去掉 app/ 前缀，另外附带一份 config/
//	├── cmd/               -> 生命周期脚本，原样放在包根
//	├── config/            -> privilege / resource
//	├── manifest
//	├── wizard/
//	└── ICON.PNG / ICON_256.PNG
func unpackFPK(root *fsx.Root, stage string, r io.Reader) error {
	br := bufio.NewReaderSize(r, 1<<16)
	magic, _ := br.Peek(2)
	var tr *tar.Reader
	if len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("不是合法的 gzip 包: %w", err)
		}
		defer gz.Close()
		tr = tar.NewReader(gz)
	} else {
		tr = tar.NewReader(br)
	}

	var (
		appTgzPath string
		total      int64
		files      int
	)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			if total > 0 {
				// 已经解出内容了，说明只是尾部损坏，容忍。
				break
			}
			return fmt.Errorf("解析包失败: %w", err)
		}

		name := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if name == "." || name == "" || !filepath.IsLocal(filepath.FromSlash(name)) {
			continue // 跳过绝对路径和 ../
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue // 不还原链接，避免指向包外的符号链接
		}

		if name == "app.tgz" {
			appTgzPath = root.Abs(stage + "/app.tgz")
			if err := root.WriteFile(stage+"/app.tgz", nil, 0o644); err != nil {
				return err
			}
			f, err := os.OpenFile(appTgzPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			n, err := io.Copy(f, tr)
			f.Close()
			total += n
			if err != nil {
				return fmt.Errorf("写出 app.tgz 失败: %w", err)
			}
			continue
		}

		files++
		if files > maxImportFiles {
			return errors.New("包内文件过多，已中止")
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.Mkdir(stage+"/"+name, true); err != nil {
				return err
			}
		default:
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("读取 %s 失败: %w", name, err)
			}
			total += int64(len(data))
			if total > maxImportBytes {
				return errors.New("包体积过大，已中止")
			}
			if err := root.WriteFile(stage+"/"+name, data, 0o644); err != nil {
				return err
			}
		}
	}

	// 有些 fpk 没有 app.tgz，app/ 直接躺在包里。
	if appTgzPath == "" {
		if st, err := root.Stat(stage + "/app"); err == nil && st.Dir {
			return nil
		}
		return errors.New("包内既没有 app/ 也没有 app.tgz，可能不是 fpk")
	}

	inner, err := os.Open(appTgzPath)
	if err != nil {
		return err
	}
	defer inner.Close()
	gz, err := gzip.NewReader(inner)
	if err != nil {
		return fmt.Errorf("app.tgz 不是合法的 gzip 包: %w", err)
	}
	defer gz.Close()

	itr := tar.NewReader(gz)
	for {
		hdr, err := itr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("解析 app.tgz 失败: %w", err)
		}
		name := path.Clean(strings.TrimPrefix(hdr.Name, "./"))
		if name == "." || name == "" || !filepath.IsLocal(filepath.FromSlash(name)) {
			continue
		}
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			continue
		}
		if hdr.Typeflag == tar.TypeDir {
			if err := root.Mkdir(stage+"/app/"+name, true); err != nil {
				return err
			}
			continue
		}
		data, err := io.ReadAll(itr)
		if err != nil {
			return err
		}
		total += int64(len(data))
		if total > maxImportBytes {
			return errors.New("包体积过大，已中止")
		}
		if err := root.WriteFile(stage+"/app/"+name, data, 0o644); err != nil {
			return err
		}
	}
	_ = root.Remove(stage+"/app.tgz", false)
	return nil
}
