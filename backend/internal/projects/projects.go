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
	"sync"
	"sync/atomic"
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

// defaultPlatforms 是新建项目时预置的架构目录，对应 fpk 的 platform 值。
var defaultPlatforms = []string{"arm", "x86"}

// maxModTimeScan 是算「最后修改时间」时最多看的条目数（见 Manager.Info）。
const maxModTimeScan = 20000

// ProjectInfo 是项目列表里的一项。
type ProjectInfo struct {
	Name string `json:"name"`
	// AbsPath 是项目目录的绝对路径。界面要拿它在文件管理器里打开项目目录，
	// 而项目根目录可以被换成授权目录，拼 projectsDir + name 会拼错。
	AbsPath     string `json:"absPath"`
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

// rootsSnapshot 是「当前生效的一整套目录」，只读。
//
// 为什么要做成快照：这些值会被打包、列目录、列产物这些并发请求读取，而
// SetRoots（设置弹窗里换目录）会整套改写它们。以前它们是几个裸字段，`go test -race`
// 直接报数据竞争；更实际的问题是读方可能在两次读取之间被换掉，拿到「新的项目根 +
// 旧的项目根在数据目录里的相对路径」这种混搭——打包会照着旧位置去复制内容。
// 换成一个不可变快照 + 原子指针，读方一次拿到一整套一致的值。
type rootsSnapshot struct {
	prj    *fsx.Root // 项目根目录沙箱
	output *fsx.Root // 打包输出目录（可能在数据根之外）

	// projectRoot / outputDir 是上面两个沙箱的绝对路径。
	projectRoot string
	outputDir   string

	// prjInHome 是项目根目录相对于数据根目录的位置，用于在同一个 home 沙箱里
	// 跨目录复制；项目根目录被设置成数据目录之外的授权目录时为空，改走
	// fsx.CopyTree 的绝对路径复制。
	prjInHome string
}

// 让 HTTP 层能把「用户输入错」和「服务端故障」分开回（400/404 vs 500）。以前全归到
// 500：日志里分不出是不是故障，前端也拿不到可行动的状态码。
var (
	// ErrInvalidName 表示项目名不合法（空、带路径分隔符、..）。
	ErrInvalidName = errors.New("项目名不合法")
	// ErrNotFound 表示项目（或它的目录）不存在。
	ErrNotFound = errors.New("项目不存在")
	// ErrExists 表示同名项目已经存在。
	ErrExists = errors.New("项目已存在")
)

// cachedModTime / rememberModTime / forgetModTime 是「最后修改时间」的小缓存（见
// Manager.modTimeCache）。
func (m *Manager) cachedModTime(rel string) (string, bool) {
	m.modTimeMu.Lock()
	defer m.modTimeMu.Unlock()
	e, ok := m.modTimeCache[rel]
	if !ok || time.Since(e.at) > modTimeCacheTTL {
		return "", false
	}
	return e.mod, true
}

func (m *Manager) rememberModTime(rel, mod string) {
	m.modTimeMu.Lock()
	defer m.modTimeMu.Unlock()
	if m.modTimeCache == nil {
		m.modTimeCache = make(map[string]modTimeEntry)
	}
	m.modTimeCache[rel] = modTimeEntry{at: time.Now(), mod: mod}
}

func (m *Manager) forgetModTime(rel string) {
	m.modTimeMu.Lock()
	defer m.modTimeMu.Unlock()
	delete(m.modTimeCache, rel)
}

// Manager 是项目层的全部业务逻辑。
type Manager struct {
	cfg *config.Config
	log *logging.Logger
	fn  *fnpack.Client
	// home 是数据根目录（暂存区），启动时定下、之后不变，所以不进快照。
	home *fsx.Root

	// current 是当前生效的目录快照。读方只走 snap()，不要缓存它。
	current atomic.Pointer[rootsSnapshot]

	// handles 是所有开过的沙箱，Close 时统一释放。SetRoots 换目录时旧的不立刻
	// 关：打包是并发进行的（同一个进程里另一个 HTTP 请求正在用旧句柄），
	// 关掉会让那次打包半路 EBADF。换目录是用户手动操作，频率极低。
	handlesMu sync.Mutex
	handles   []*fsx.Root

	// modTimeCache 缓存「项目的最后修改时间」。
	//
	// 算一次要扫整个项目树（上限 maxModTimeScan 项），而首页每次刷新 / 新建项目都会把
	// 每个项目扫一遍——项目一多，/api/projects 就是「项目数 × 最多两万次 stat」。这里
	// 做很短的 TTL 复用：连续刷新不会重复扫，代价是最多 modTimeCacheTTL 的显示延迟。
	// 会改项目内容的操作（打包、新建、删除）顺手把对应键丢掉。
	modTimeMu    sync.Mutex
	modTimeCache map[string]modTimeEntry
}

// modTimeEntry 是一条「最后修改时间」缓存。
type modTimeEntry struct {
	at  time.Time
	mod string
}

// modTimeCacheTTL 见 Manager.modTimeCache 的说明。
const modTimeCacheTTL = 2 * time.Second

// snap 返回当前生效的目录快照（一次原子读，拿到的一定是同一套值）。
func (m *Manager) snap() *rootsSnapshot { return m.current.Load() }

// prjRoot / outputRoot 是给内部读写用的便捷入口（每个调用点各拿一次快照）。
func (m *Manager) prjRoot() *fsx.Root    { return m.snap().prj }
func (m *Manager) outputRoot() *fsx.Root { return m.snap().output }

// New 创建 Manager，先按环境变量给的默认目录打开沙箱。
// 用户设置过的目录由 SetRoots 在启动时补一次。
func New(cfg *config.Config, log *logging.Logger, fn *fnpack.Client) (*Manager, error) {
	home, err := fsx.Open(cfg.DataDir, true)
	if err != nil {
		return nil, fmt.Errorf("打开数据目录失败: %w", err)
	}
	m := &Manager{cfg: cfg, log: log, fn: fn, home: home}
	m.handles = []*fsx.Root{home}
	if err := m.SetRoots("", ""); err != nil {
		return nil, err
	}
	return m, nil
}

// Close 释放句柄。要在服务退出时调（此时不会再有请求进来）。
func (m *Manager) Close() {
	m.handlesMu.Lock()
	defer m.handlesMu.Unlock()
	for _, r := range m.handles {
		if r != nil {
			_ = r.Close()
		}
	}
	m.handles = nil
}

// ProjectRoot 返回当前生效的项目根目录。
func (m *Manager) ProjectRoot() string { return m.snap().projectRoot }

// OutputDir 返回当前生效的打包输出目录。
func (m *Manager) OutputDir() string { return m.snap().outputDir }

// SetRoots 切换项目根目录与打包输出目录，空字符串表示用环境变量给的默认值。
//
// 允许把项目根目录指到数据目录之外（用户授权给应用的目录），这正是这个能力的
// 意义所在：项目本身待在用户自己的存储空间里，数据目录只放日志、暂存和配置。
// 校验只做「能不能用」这一层（必须是绝对路径、必须是已存在的目录、不能是根目录
// 或数据目录本身），不做白名单——授权范围由飞牛系统的文件选择器与 ACL 决定。
func (m *Manager) SetRoots(projectRoot, outputDir string) error {
	projectRoot = strings.TrimSpace(projectRoot)
	outputDir = strings.TrimSpace(outputDir)

	prjDir := m.cfg.ProjectsDir
	if projectRoot != "" {
		if !filepath.IsAbs(projectRoot) {
			return fmt.Errorf("项目根目录必须是绝对路径：%s", projectRoot)
		}
		abs, err := filepath.Abs(projectRoot)
		if err != nil {
			return fmt.Errorf("项目根目录不是合法路径: %w", err)
		}
		abs = filepath.Clean(abs)
		if abs == string(filepath.Separator) {
			return errors.New("项目根目录不能是文件系统根目录")
		}
		if abs == filepath.Clean(m.cfg.DataDir) {
			return errors.New("项目根目录不能是数据目录（日志、暂存都在里面）")
		}
		st, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("项目根目录不可用: %w", err)
		}
		if !st.IsDir() {
			return fmt.Errorf("项目根目录不是目录：%s", abs)
		}
		prjDir = abs
	}

	outDir := m.cfg.OutputDir
	if outputDir != "" {
		if !filepath.IsAbs(outputDir) {
			return fmt.Errorf("输出目录必须是绝对路径：%s", outputDir)
		}
		abs, err := filepath.Abs(outputDir)
		if err != nil {
			return fmt.Errorf("输出目录不是合法路径: %w", err)
		}
		abs = filepath.Clean(abs)
		if abs == string(filepath.Separator) {
			return errors.New("输出目录不能是文件系统根目录")
		}
		if abs == filepath.Clean(m.cfg.DataDir) {
			return errors.New("输出目录不能是数据目录（日志、暂存都在里面）")
		}
		outDir = abs
	}
	// 两个参数都为空表示「用环境变量给的默认值」，这时不校验两者是否相同：
	// 默认值可能本来就撞在一起（项目目录被指到 $DATA_DIR/output，而输出目录的默认
	// 值正是它），挡下来等于服务直接起不来。这时复制到输出目录本来就等于复制到
	// 自己身上，交给 outputIsProject 兜住（不重复复制、不重复列产物）。
	// 用户明确指定的值仍然严格校验——界面上两个槽不允许选同一个目录。
	sameIsDefault := strings.TrimSpace(projectRoot) == "" && strings.TrimSpace(outputDir) == ""
	if !sameIsDefault && samePath(outDir, prjDir) {
		return errors.New("输出目录不能和项目根目录相同")
	}

	prj, err := fsx.Open(prjDir, true)
	if err != nil {
		return fmt.Errorf("打开项目根目录失败: %w", err)
	}
	out, err := fsx.Open(outDir, true)
	if err != nil {
		_ = prj.Close()
		return fmt.Errorf("打开输出目录失败: %w", err)
	}

	// 项目根落在数据目录内时，暂存复制可以在 home 沙箱里用相对路径完成；
	// 落在外面（授权目录）就只能按绝对路径复制。
	inHome := ""
	if rel, err := filepath.Rel(m.cfg.DataDir, prjDir); err == nil && !strings.HasPrefix(rel, "..") {
		inHome = filepath.ToSlash(rel)
	} else {
		m.log.Debugf("项目根目录在数据目录之外（%s），暂存复制改走绝对路径", prjDir)
	}

	m.handlesMu.Lock()
	m.handles = append(m.handles, prj, out)
	m.handlesMu.Unlock()

	// 一次性换掉整套目录：读方（打包 / 列目录 / 列产物）拿到的永远是一整套一致的
	// 值。校验失败的分支都在上面 return 了，走到这里就一定可以换。
	m.current.Store(&rootsSnapshot{
		prj:         prj,
		output:      out,
		projectRoot: prjDir,
		outputDir:   outDir,
		prjInHome:   inHome,
	})
	return nil
}

// samePath 判断两个路径是不是同一个位置。
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

// outputIsProject 判断输出目录是不是就是项目根目录。
//
// 正常部署下不会相同（配置里会把重了的输出目录退回默认值，界面上也拒绝两个槽选同
// 一个目录），但默认值仍可能撞上：项目目录可以被指到数据目录下的任何位置，包括
// $DATA_DIR/output。那时复制到输出目录等于复制到自己身上，列产物也会列两遍。
func (m *Manager) outputIsProject() bool {
	s := m.snap()
	return filepath.Clean(s.outputDir) == filepath.Clean(s.projectRoot)
}

// copyIntoStage 把项目目录复制到 home 沙箱里的暂存位置。
//
// 项目根目录在数据目录内时走 home 沙箱的相对路径复制；被设置成数据目录之外的
// 授权目录时，源和目标分属两个沙箱，只能按绝对路径复制。
//
// 快照由调用方给（s）：这个操作要读两个目录字段，自己各读一次的话，正好在两次读之间
// 换过目录就会拿到混搭的值。
func (m *Manager) copyIntoStage(s *rootsSnapshot, prjRel, stageRel string) error {
	if s.prjInHome != "" {
		return m.home.Copy(s.prjInHome+"/"+prjRel, stageRel)
	}
	return fsx.CopyTree(s.prj.Abs(prjRel), m.home.Abs(stageRel))
}

// Root 暴露项目目录沙箱，供文件管理器复用。
func (m *Manager) Root() *fsx.Root { return m.prjRoot() }

// List 列出全部项目。
func (m *Manager) List() ([]ProjectInfo, error) {
	entries, err := m.prjRoot().List(".")
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
			out = append(out, ProjectInfo{Name: e.Name, ModTime: e.ModTime, AbsPath: m.prjRoot().Abs(e.Name)})
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
	st, err := m.prjRoot().Stat(rel)
	if err != nil {
		return ProjectInfo{}, err
	}
	if !st.Dir {
		return ProjectInfo{}, fmt.Errorf("%w：%s 不是目录", ErrNotFound, name)
	}

	info := ProjectInfo{Name: name, ModTime: st.ModTime, AbsPath: m.prjRoot().Abs(rel)}
	// 列表里显示的是「最后修改时间」：目录自身的 mtime 只在增删条目时变，
	// 改 manifest、换 building 里的产物都看不出来，所以取整棵树里最新的。
	// 限制扫描条目数，避免大项目把一个时间扫穿；结果再放 2 秒缓存（见 modTimeCache）。
	if cached, ok := m.cachedModTime(rel); ok {
		info.ModTime = cached
	} else if newest, err := m.prjRoot().LatestModTime(rel, maxModTimeScan); err == nil && !newest.IsZero() {
		info.ModTime = newest.Format("2006-01-02 15:04:05")
		m.rememberModTime(rel, info.ModTime)
	}
	if data, err := m.prjRoot().ReadFile(rel + "/manifest"); err == nil {
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
	st, err := m.prjRoot().Stat(rel)
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
	if _, err := m.prjRoot().Stat(rel); err == nil {
		return fmt.Errorf("%w：%s", ErrExists, name)
	}

	out, err := m.fn.Create(ctx, m.snap().projectRoot, name, docker, withUI)
	if err != nil {
		return fmt.Errorf("fnpack create 失败: %w", err)
	}
	// fnpack create 失败时退出码仍然是 0（错误只打在 stdout 上），所以这里以
	// 「目录 / manifest 有没有真的落地」为判据，并把 fnpack 的原话带出来——
	// 否则用户只会看到一句没头没脑的「报告成功但目录没生成」，不知道是自己
	// 项目名不合规。
	if _, err := m.prjRoot().Stat(rel); err != nil {
		if reason := fnpack.FailureReason(out); reason != "" {
			return fmt.Errorf("fnpack create 失败: %s", reason)
		}
		return fmt.Errorf("fnpack create 没有生成项目目录 %s，请查看日志里 fnpack 的输出", rel)
	}
	if !m.exists(rel + "/manifest") {
		return fmt.Errorf("fnpack create 没有生成项目骨架（缺少 %s/manifest）", rel)
	}

	// fnpack 模板里带了 .DS_Store，在部分文件系统上会让后续 build 直接失败。
	if n := stripJunk(m.prjRoot(), rel); n > 0 {
		m.log.Infof("清理模板里的冗余文件 %d 个", n)
	}
	// 这里刻意不修模式位（打包时会修暂存副本）：fnpack 新建的文件在飞牛卷上
	// 可能没有执行位，但那是「产物要不要可执行」的问题，Pack 前会统一修一次；
	// 在此处 chmod 反而会把项目目录的 ACL 换成模式位，动到用户的文件权限。
	// 建工作目录：building/platform 下预置 arm / x86，多架构发布勾一下就能用
	// （架构目录名就是 fpk 的 platform 值，内容会被复制到 app/server 再打包）。
	dirs := append([]string{rel + "/building", rel + "/building/platform"}, platformDirs(rel)...)
	for _, d := range dirs {
		if err := m.prjRoot().Mkdir(d, 0o755, true); err != nil {
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
	if _, err := m.prjRoot().Stat(rel); err != nil {
		return fmt.Errorf("%w：%s", ErrNotFound, name)
	}
	defer m.forgetModTime(rel)
	return m.prjRoot().Remove(rel, true)
}

// Platforms 列出 building/platform 下的架构目录。
func (m *Manager) Platforms(name string) ([]string, error) {
	rel, err := projectPath(name)
	if err != nil {
		return nil, err
	}
	entries, err := m.prjRoot().List(rel + "/building/platform")
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
	if data, err := m.prjRoot().ReadFile(rel + "/manifest"); err == nil {
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
	s := m.snap()
	collect(s.prj, rel, "project", nil)
	// 两个目录是同一个位置时只列项目那一份：否则同一份产物会以两种 scope 出现，
	// 用户看到的是「列表里有一个文件出现两次，路径一模一样」。
	if !m.outputIsProject() {
		collect(s.output, ".", "output", func(n string) bool {
			return strings.HasPrefix(n, appname+"-") || n == appname+".fpk"
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ModTime > out[j].ModTime })
	return out, nil
}

// GetSettings 读取打包偏好，文件不存在时返回默认值。
func (m *Manager) GetSettings(name string) (Settings, error) {
	rel, err := projectPath(name)
	if err != nil {
		return Settings{}, err
	}
	s := Settings{ServerDir: defaultServerDir, CopyToOutput: true, Platforms: []string{}}
	data, err := m.prjRoot().ReadFile(rel + "/" + settingsFile)
	if err != nil {
		return s, nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, nil
	}
	if s.ServerDir == "" {
		s.ServerDir = defaultServerDir
	}
	return normalizeSettings(s), nil
}

// normalizeSettings 把 nil 切片补成空切片。
//
// Go 的 nil slice 会被编码成 `null`，前端拿到 `null` 再当数组用（展开、遍历）
// 会直接抛异常——表现是「设置里的架构/落地目录读不到，页面中途断掉」。
func normalizeSettings(s Settings) Settings {
	if s.Platforms == nil {
		s.Platforms = []string{}
	}
	return s
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
	data, err := json.MarshalIndent(normalizeSettings(s), "", "  ")
	if err != nil {
		return err
	}
	return m.prjRoot().WriteFile(rel+"/"+settingsFile, data, 0o644)
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
	manifestBytes, err := m.prjRoot().ReadFile(rel + "/manifest")
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

	// 这一次打包全程用同一套目录（见 rootsSnapshot 的说明）。
	s := m.snap()

	stageName := fmt.Sprintf("build-%s-%d", sanitize(req.Name), time.Now().UnixNano())
	stage := "tmp/" + stageName
	if err := m.home.Mkdir(stage, 0o755, true); err != nil {
		return nil, fmt.Errorf("创建暂存目录失败: %w", err)
	}
	defer func() {
		if err := m.home.Remove(stage, true); err != nil {
			m.log.Warnf("清理暂存目录失败: %v", err)
		}
	}()

	if err := m.copyIntoStage(s, rel, stage+"/project"); err != nil {
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

	m.log.Infof("开始打包 %s (%s)，输出目录 %s", req.Name, version, s.outputDir)

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
			if err := m.prepareStage(s, stage, rel, serverRel, platform); err != nil {
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
		inProject := m.prjRoot().Abs(rel + "/" + fileName)
		if err := copyFile(src, inProject); err != nil {
			res.OK = false
			res.Log = fmt.Sprintf("写入项目目录失败: %v", err)
			results = append(results, res)
			continue
		}
		res.File = fileName
		res.Path = inProject
		res.Size = st.Size()

		if req.CopyOut && !m.outputIsProject() {
			dst := m.outputRoot().Abs(fileName)
			if err := copyFile(src, dst); err != nil {
				res.OK = false
				res.Log = fmt.Sprintf("复制到输出目录失败: %v", err)
				results = append(results, res)
				continue
			}
			res.Path = dst
		}

		res.OK = true
		// 产物写进了项目目录，列表上的「最后修改时间」要跟着变（缓存里那份作废）
		m.forgetModTime(rel)
		m.log.Infof("打包成功 %s（%s）", fileName, fsx.HumanSize(st.Size()))
		results = append(results, res)
	}
	return results, nil
}

// prepareStage 把 building/platform/<platform>/ 的内容覆盖到暂存区的 server 目录。
//
// 约定与 fnpackup 一致：架构目录里放的是「该架构下 app/<serverDir> 应有的全部内容」。
func (m *Manager) prepareStage(s *rootsSnapshot, stage string, prjRel string, serverRel string, platform string) error {
	srcInPrj := prjRel + "/building/platform/" + platform

	srcStat, err := m.prjRoot().Stat(srcInPrj)
	if err != nil {
		return fmt.Errorf("找不到架构目录 building/platform/%s", platform)
	}
	if !srcStat.Dir {
		return fmt.Errorf("building/platform/%s 不是目录", platform)
	}
	children, err := m.prjRoot().List(srcInPrj)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		m.log.Warnf("架构目录 building/platform/%s 是空的，将以空 server 目录打包", platform)
	}

	dstRel := stage + "/project/" + serverRel
	// 目标可能不存在（fnpack 只创建 app/server 与 app/www），不存在的错误直接忽略。
	_ = m.home.Remove(dstRel, true)
	if err := m.home.Mkdir(dstRel, 0o755, true); err != nil {
		return err
	}
	return m.copyIntoStage(s, srcInPrj, dstRel)
}

// Import 把上传的 .fpk 解成一个项目目录。
func (m *Manager) Import(ctx context.Context, r io.Reader, fileName string) (ProjectInfo, error) {
	stage := fmt.Sprintf("tmp/import-%d", time.Now().UnixNano())
	if err := m.home.Mkdir(stage, 0o755, true); err != nil {
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
	if _, err := m.prjRoot().Stat(rel); err == nil {
		name = fmt.Sprintf("%s-%d", name, time.Now().Unix())
		rel, err = projectPath(name)
		if err != nil {
			return ProjectInfo{}, err
		}
	}

	if err := m.moveIntoProjects(stage, rel); err != nil {
		return ProjectInfo{}, err
	}
	// 导入的 fpk 里没有多架构工作目录（包内只有打包后的 app/），这里补上 arm / x86：
	// 与 Create 保持一致，多架构发布勾一下就能用。失败只记日志——项目已经落地了，
	// 而且前端的「打开多架构发布」还会再补一次。
	for _, dir := range platformDirs(rel) {
		if err := m.prjRoot().Mkdir(dir, 0o755, true); err != nil {
			m.log.Warnf("创建架构目录 %s 失败: %v", dir, err)
		}
	}

	m.log.Infof("导入项目 %s 完成", name)
	return m.Info(name)
}

// ImportPath 从本机的一个绝对路径导入 fpk。
//
// 路径来自飞牛文件选择器（前端 pickUserFile）：系统已经把这个文件授权给应用
// 用户，所以应用进程读得到。它不在项目沙箱里，因此这里用 os.Open，而不是
// fsx.Root——沙箱是给「用户项目目录」用的，不是给任意路径用的。
//
// 权限仍然按当前登录用户判一遍（调用方负责，见 server.handleProjectImportPath）：
// 授权让应用用户拿到了 ACL，不代表当前登录用户也能读这个文件。
func (m *Manager) ImportPath(ctx context.Context, absPath string) (ProjectInfo, error) {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return ProjectInfo{}, errors.New("缺少文件路径")
	}
	if !filepath.IsAbs(absPath) {
		return ProjectInfo{}, fmt.Errorf("需要绝对路径：%s", absPath)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("读取文件状态失败: %w", err)
	}
	if fi.IsDir() {
		return ProjectInfo{}, fmt.Errorf("这是一个目录，不是 fpk 文件：%s", absPath)
	}

	info, err := m.Import(ctx, f, filepath.Base(absPath))
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("导入 %s 失败: %w", filepath.Base(absPath), err)
	}
	return info, nil
}

// moveIntoProjects 把暂存出来的项目搬到项目根目录。
//
// 优先 rename（同文件系统时是瞬时的）；项目根目录可以是另一个挂载点上的授权
// 目录，跨设备 rename 会 EXDEV，那时退回复制 + 删源。
func (m *Manager) moveIntoProjects(stage, rel string) error {
	src := m.home.Abs(stage)
	dst := m.prjRoot().Abs(rel)

	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	if copyErr := fsx.CopyTree(src, dst); copyErr != nil {
		return fmt.Errorf("移动到项目目录失败: %v（尝试复制也没成功: %w）", err, copyErr)
	}
	if rmErr := m.home.Remove(stage, true); rmErr != nil {
		m.log.Warnf("清理导入暂存目录失败: %v", rmErr)
	}
	return nil
}

// platformDirs 是某个项目下预置的架构工作目录（相对项目根）。
func platformDirs(rel string) []string {
	dirs := make([]string, 0, len(defaultPlatforms))
	for _, arch := range defaultPlatforms {
		dirs = append(dirs, rel+"/building/platform/"+arch)
	}
	return dirs
}

// ---- 内部工具 ----

func (m *Manager) exists(rel string) bool {
	_, err := m.prjRoot().Stat(rel)
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
		return "", fmt.Errorf("%w：项目名不能为空", ErrInvalidName)
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return "", fmt.Errorf("%w：项目名不能包含路径分隔符", ErrInvalidName)
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

// writeEntry 把归档里的一个成员**流式**写进暂存目录，并把它计入体积预算。
//
// 以前这里是 io.ReadAll：导入一个几 GB 的 fpk 会在内存里堆出同样大的一份，而导入
// 的来源（上传或本机路径）本身没有大小上限。边读边写之后内存是常数级的，代价是预算
// 只能在读的过程中判——多读一个字节就报错（多读的那一个字节用来区分「正好用满」和
// 「超了」）。写了一半的成员不用清：调用方会把整个暂存目录删掉。
func writeEntry(root *fsx.Root, rel string, r io.Reader, total *int64) error {
	remaining := maxImportBytes - *total
	if remaining <= 0 {
		return errors.New("包体积过大，已中止")
	}
	n, err := root.WriteReader(rel, io.LimitReader(r, remaining+1), 0o644)
	*total += n
	if err != nil {
		return fmt.Errorf("写出 %s 失败: %w", rel, err)
	}
	if n > remaining {
		return errors.New("包体积过大，已中止")
	}
	return nil
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
			// app.tgz 是包内最大的那个成员，同样要计进体积预算——以前这里是裸的
			// io.Copy，一个超大的 app.tgz 会被整份写出去才轮到后面的检查。
			n, err := io.Copy(f, io.LimitReader(tr, maxImportBytes-total+1))
			f.Close()
			total += n
			if err != nil {
				return fmt.Errorf("写出 app.tgz 失败: %w", err)
			}
			if total > maxImportBytes {
				return errors.New("包体积过大，已中止")
			}
			continue
		}

		files++
		if files > maxImportFiles {
			return errors.New("包内文件过多，已中止")
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.Mkdir(stage+"/"+name, 0o755, true); err != nil {
				return err
			}
		default:
			if err := writeEntry(root, stage+"/"+name, tr, &total); err != nil {
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
			if err := root.Mkdir(stage+"/app/"+name, 0o755, true); err != nil {
				return err
			}
			continue
		}
		// 文件数也要算上 app.tgz 里那些：体积预算对**空文件**不起作用，只按字节数拦的话，
		// 一个塞满几百万个 0 字节成员的 app.tgz 能把数据目录的 inode 吃光。
		files++
		if files > maxImportFiles {
			return errors.New("包内文件过多，已中止")
		}
		if err := writeEntry(root, stage+"/app/"+name, itr, &total); err != nil {
			return err
		}
	}
	_ = root.Remove(stage+"/app.tgz", false)
	return nil
}
