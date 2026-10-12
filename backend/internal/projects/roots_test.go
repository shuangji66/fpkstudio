package projects

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
)

// newTestManager 造一个数据目录在 tmp 里的 Manager（默认项目根目录在数据目录内）。
func newTestManager(t *testing.T) (*Manager, *config.Config) {
	t.Helper()

	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir:     dataDir,
		ProjectsDir: filepath.Join(dataDir, "projects"),
		OutputDir:   filepath.Join(dataDir, "output"),
	}
	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	m, err := New(cfg, log, fnpack.New(cfg, log))
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}
	t.Cleanup(m.Close)
	return m, cfg
}

// 项目根目录可以被指到数据目录之外（用户授权的目录），这正是设置弹窗里
// 「设置为项目根目录」要做的事。
func TestSetRootsOutsideDataDir(t *testing.T) {
	m, cfg := newTestManager(t)

	outside := filepath.Join(t.TempDir(), "授权 目录")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	outDir := filepath.Join(t.TempDir(), "out")

	if err := m.SetRoots(outside, outDir); err != nil {
		t.Fatalf("切换目录失败: %v", err)
	}
	if m.ProjectRoot() != outside {
		t.Errorf("ProjectRoot() = %q，期望 %q", m.ProjectRoot(), outside)
	}
	if m.OutputDir() != outDir {
		t.Errorf("OutputDir() = %q，期望 %q", m.OutputDir(), outDir)
	}
	// 数据目录之外时没有 home 相对路径，暂存复制要走绝对路径。
	if m.snap().prjInHome != "" {
		t.Errorf("prjInHome = %q，期望为空", m.snap().prjInHome)
	}
	if _, err := os.Stat(outDir); err != nil {
		t.Errorf("输出目录应当被创建: %v", err)
	}

	// 换回默认值：空字符串表示用环境变量给的目录。
	if err := m.SetRoots("", ""); err != nil {
		t.Fatalf("恢复默认目录失败: %v", err)
	}
	if m.ProjectRoot() != cfg.ProjectsDir || m.OutputDir() != cfg.OutputDir {
		t.Errorf("恢复默认后 = %q / %q", m.ProjectRoot(), m.OutputDir())
	}
}

// 校验只做「能不能用」这一层：相对路径、不存在的目录、根目录、数据目录本身、
// 输出目录与项目根相同，都要挡住。
func TestSetRootsRejectsBadInput(t *testing.T) {
	m, cfg := newTestManager(t)
	dir := t.TempDir()
	// 一个真实存在但会被用来当「不存在的目录」的相对路径
	cases := []struct {
		name        string
		projectRoot string
		outputDir   string
	}{
		{"相对路径", "relative/dir", ""},
		{"项目根不存在", filepath.Join(dir, "nope"), ""},
		{"项目根是文件", filepath.Join(dir, "file"), ""},
		{"项目根是根目录", string(filepath.Separator), ""},
		{"项目根是数据目录", cfg.DataDir, ""},
		{"输出目录与项目根相同", cfg.ProjectsDir, cfg.ProjectsDir},
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	for _, c := range cases {
		if err := m.SetRoots(c.projectRoot, c.outputDir); err == nil {
			t.Errorf("%s：期望报错，实际通过", c.name)
		}
	}
	// 全部失败之后，生效的目录必须还是原来那一套（不能改一半）。
	if m.ProjectRoot() != cfg.ProjectsDir || m.OutputDir() != cfg.OutputDir {
		t.Errorf("校验失败后目录被改动了：%q / %q", m.ProjectRoot(), m.OutputDir())
	}
}

// 从本机绝对路径导入 fpk：路径在项目沙箱之外，且改名跨挂载点时要退回复制。
func TestImportPathOutsideProjectRoot(t *testing.T) {
	m, _ := newTestManager(t)

	srcDir := t.TempDir()
	fpkPath := filepath.Join(srcDir, "demo.fpk")
	if err := os.WriteFile(fpkPath, fakeFPK(t), 0o644); err != nil {
		t.Fatalf("写 fpk 失败: %v", err)
	}

	info, err := m.ImportPath(context.Background(), fpkPath)
	if err != nil {
		t.Fatalf("按路径导入失败: %v", err)
	}
	if info.Name != "demo" {
		t.Fatalf("项目名 = %q，期望 demo", info.Name)
	}
	if _, err := m.prjRoot().Stat("demo/manifest"); err != nil {
		t.Errorf("项目没有落到项目根目录: %v", err)
	}
	// 导入后照例补上多架构工作目录。
	if _, err := m.prjRoot().Stat("demo/building/platform/arm"); err != nil {
		t.Errorf("缺少架构目录: %v", err)
	}
}

func TestImportPathRejectsBadInput(t *testing.T) {
	m, _ := newTestManager(t)

	if _, err := m.ImportPath(context.Background(), ""); err == nil {
		t.Error("空路径应当报错")
	}
	if _, err := m.ImportPath(context.Background(), "relative/file.fpk"); err == nil {
		t.Error("相对路径应当报错")
	}
	if _, err := m.ImportPath(context.Background(), t.TempDir()); err == nil {
		t.Error("目录应当报错")
	}
	if _, err := m.ImportPath(context.Background(), filepath.Join(t.TempDir(), "nope.fpk")); err == nil {
		t.Error("不存在的文件应当报错")
	}

	bad := filepath.Join(t.TempDir(), "bad.fpk")
	if err := os.WriteFile(bad, []byte("这不是一个 fpk"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	if _, err := m.ImportPath(context.Background(), bad); err == nil {
		t.Error("不是 fpk 的文件应当报错")
	}
}

// 配置能挡住的只是「两个环境变量填重了」；项目目录被指到 $DATA_DIR/output 时，
// 输出目录的默认值正好也是它，回退之后还是同一个。这时复制到输出目录等于复制到
// 自己身上、产物列表还会把同一个文件列两遍，所以各有一道兜底。
func TestOutputSameAsProjectRoot(t *testing.T) {
	dataDir := t.TempDir()
	out := filepath.Join(dataDir, "output")
	cfg := &config.Config{DataDir: dataDir, ProjectsDir: out, OutputDir: out}

	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatalf("创建日志器失败: %v", err)
	}
	defer log.Close()

	m, err := New(cfg, log, fnpack.New(cfg, log))
	if err != nil {
		t.Fatalf("创建 Manager 失败: %v", err)
	}
	defer m.Close()

	if !m.outputIsProject() {
		t.Fatalf("项目根 %q 与输出目录 %q 相同，outputIsProject() 应为真", m.ProjectRoot(), m.OutputDir())
	}

	// 造一个项目与一份产物（就放在这个既是项目目录又是输出目录的地方）。
	if err := m.prjRoot().Mkdir("demo", 0o755, true); err != nil {
		t.Fatalf("创建项目失败: %v", err)
	}
	if err := m.prjRoot().WriteFile("demo/manifest", []byte("appname               = demo\nversion               = 1.0.0\nplatform              = x86\n"), 0o644); err != nil {
		t.Fatalf("写 manifest 失败: %v", err)
	}
	if err := m.prjRoot().WriteFile("demo/demo-1.0.0-x86.fpk", []byte("x"), 0o644); err != nil {
		t.Fatalf("写产物失败: %v", err)
	}

	list, err := m.Artifacts("demo")
	if err != nil {
		t.Fatalf("列产物失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("产物数 = %d（%v），期望 1：两个目录是同一个位置时不能列两遍", len(list), list)
	}
	if list[0].Scope != "project" {
		t.Errorf("scope = %q，期望 project", list[0].Scope)
	}
}

// 默认值撞在一起时不能拒绝（否则服务直接起不来），但用户明确指定的相同值要拒绝。
func TestSetRootsRejectsSamePathFromUser(t *testing.T) {
	m, cfg := newTestManager(t)
	dir := t.TempDir()

	if err := m.SetRoots(dir, dir); err == nil {
		t.Error("用户明确把两个目录设成同一个应当报错")
	}
	// 只改一个、另一个沿用默认值：这时两个值仍然不同，应当通过。
	if err := m.SetRoots(dir, ""); err != nil {
		t.Errorf("只改项目目录不应报错: %v", err)
	}
	if m.ProjectRoot() != dir || m.OutputDir() != cfg.OutputDir {
		t.Errorf("切换结果 = %q / %q", m.ProjectRoot(), m.OutputDir())
	}
}
