package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
)

// fakeFnpack 造一个假的 fnpack 可执行文件。
//
// 真 fnpack 不在构建机上，而 Pack 的绝大多数分支（暂存、架构目录替换、产物落地）
// 都在调它之前或之后，用假脚本就能覆盖到；脚本还会把暂存区 app/server 下的文件清单
// 写进 $FPKSTUDIO_TEST_STAGE，用来断言「架构目录真的替换了 server 目录」。
func fakeFnpack(t *testing.T, appname string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fnpack")
	script := `#!/bin/sh
if [ "$1" = "build" ]; then
  find app/server -type f | sort > "$FPKSTUDIO_TEST_STAGE"
  printf 'fake-fpk' > ` + appname + `.fpk
fi
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// newPackManager 造一个「项目根在数据目录内」的默认部署（prjInHome != ""），并把
// fnpack 换成假脚本——Pack 要跑到子进程这一步，真 fnpack 不在构建机上。
func newPackManager(t *testing.T) (*Manager, *config.Config) {
	t.Helper()
	data := t.TempDir()
	cfg := &config.Config{
		DataDir:     data,
		ProjectsDir: filepath.Join(data, "projects"),
		OutputDir:   filepath.Join(data, "output"),
		TmpDir:      filepath.Join(data, "tmp"),
		FnpackBin:   fakeFnpack(t, "demo"),
	}
	log, err := logging.New("", "debug", 100)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(cfg, log, fnpack.New(cfg, log))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, cfg
}

// seedProject 造一个带两个架构目录的项目。
func seedProject(t *testing.T, root string) {
	t.Helper()
	seedProjectWithManifest(t, root, "appname=demo\nversion=1.0.0\nplatform=x86\n")
}

// seedProjectWithManifest 同上，但 manifest 内容由调用方给（用来构造畸形 manifest）。
func seedProjectWithManifest(t *testing.T, root, manifest string) {
	t.Helper()
	proj := filepath.Join(root, "demo")
	files := map[string]string{
		"manifest":                       manifest,
		"app/server/only-in-project.txt": "x86 的残留，替换语义下不该出现在 arm 包里\n",
		"building/platform/arm/main":     "arm-bin\n",
		"building/platform/x86/main":     "x86-bin\n",
		"building/platform/arm/lib/a.so": "arm-lib\n",
		"building/platform/x86/lib/a.so": "x86-lib\n",
		"app/www/index.html":             "<html></html>\n",
		"app/ui/config":                  "{\".url\":{}}\n",
		"config/privilege":               "{}\n",
	}
	for rel, body := range files {
		full := filepath.Join(proj, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestPackMultiPlatformDefaultDeploy 是 S1 的回归：默认部署（项目根在数据目录内，
// copyIntoStage 走 home 沙箱的 Root.Copy）下，多架构打包必须真的能出包。
//
// 以前的 prepareStage 先 Mkdir 出目标目录，再调对「已存在目标」返回 ErrExists 的
// Root.Copy —— 每个架构都失败，界面上只看到一句「目标已存在」。
func TestPackMultiPlatformDefaultDeploy(t *testing.T) {
	m, _ := newPackManager(t)
	seedProject(t, m.ProjectRoot())

	stageDump := filepath.Join(t.TempDir(), "stage.txt")
	t.Setenv("FPKSTUDIO_TEST_STAGE", stageDump)

	results, err := m.Pack(context.Background(), PackRequest{
		Name:      "demo",
		Platforms: []string{"arm", "x86"},
		ServerDir: "app/server",
	})
	if err != nil {
		t.Fatalf("Pack 返回错误: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("期望 2 个架构结果，得到 %d 个: %+v", len(results), results)
	}
	for _, res := range results {
		if !res.OK {
			t.Errorf("架构 %s 打包失败：%s", res.Platform, res.Log)
		}
		if res.File == "" {
			t.Errorf("架构 %s 没有产物名", res.Platform)
		}
	}
	for _, name := range []string{"demo-1.0.0-arm.fpk", "demo-1.0.0-x86.fpk"} {
		if _, err := os.Stat(filepath.Join(m.ProjectRoot(), "demo", name)); err != nil {
			t.Errorf("产物 %s 没有落在项目目录里: %v", name, err)
		}
	}

	// 架构目录的内容必须整份替换掉 app/server（而不是合并）：
	// 否则 x86 的残留文件会跟着打进 arm 包。
	body, err := os.ReadFile(stageDump)
	if err != nil {
		t.Fatalf("读取暂存区清单失败: %v", err)
	}
	got := string(body)
	if strings.Contains(got, "only-in-project.txt") {
		t.Errorf("替换语义被破坏：项目里原有的 app/server 文件还在暂存区里\n%s", got)
	}
	if !strings.Contains(got, "app/server/main") || !strings.Contains(got, "app/server/lib/a.so") {
		t.Errorf("架构目录的内容没有复制进 app/server：\n%s", got)
	}
}

// TestPackMultiPlatformExternalRoot 覆盖另一条复制路径（项目根在数据目录之外时走
// fsx.CopyTree）：它以前就是好的，这里钉住别在修 S1 时改坏。
func TestPackMultiPlatformExternalRoot(t *testing.T) {
	m, _ := newPackManager(t)
	outside := t.TempDir()
	prjRoot := filepath.Join(outside, "user-projects")
	if err := os.MkdirAll(prjRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.SetRoots(prjRoot, ""); err != nil {
		t.Fatal(err)
	}
	seedProject(t, m.ProjectRoot())
	t.Setenv("FPKSTUDIO_TEST_STAGE", filepath.Join(t.TempDir(), "stage.txt"))

	results, err := m.Pack(context.Background(), PackRequest{
		Name:      "demo",
		Platforms: []string{"arm", "x86"},
		ServerDir: "app/server",
	})
	if err != nil {
		t.Fatalf("Pack 返回错误: %v", err)
	}
	for _, res := range results {
		if !res.OK {
			t.Errorf("架构 %s 打包失败：%s", res.Platform, res.Log)
		}
	}
}

// TestPackSinglePlatform 单包（沿用 manifest 里的 platform）也要能出包。
func TestPackSinglePlatform(t *testing.T) {
	m, _ := newPackManager(t)
	seedProject(t, m.ProjectRoot())
	t.Setenv("FPKSTUDIO_TEST_STAGE", filepath.Join(t.TempDir(), "stage.txt"))

	results, err := m.Pack(context.Background(), PackRequest{Name: "demo"})
	if err != nil {
		t.Fatalf("Pack 返回错误: %v", err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("单包失败: %+v", results)
	}
	if results[0].Platform != "x86" {
		t.Errorf("单包的 platform 应当沿用 manifest 里的值，得到 %q", results[0].Platform)
	}
	if _, err := os.Stat(filepath.Join(m.ProjectRoot(), "demo", "demo-1.0.0-x86.fpk")); err != nil {
		t.Errorf("单包产物没有落地: %v", err)
	}
}

// TestPackMissingPlatformDir 架构目录不存在时只让那一个架构失败，不影响其它架构。
func TestPackMissingPlatformDir(t *testing.T) {
	m, _ := newPackManager(t)
	seedProject(t, m.ProjectRoot())
	t.Setenv("FPKSTUDIO_TEST_STAGE", filepath.Join(t.TempDir(), "stage.txt"))

	results, err := m.Pack(context.Background(), PackRequest{
		Name:      "demo",
		Platforms: []string{"arm", "mips"},
		ServerDir: "app/server",
	})
	if err != nil {
		t.Fatalf("Pack 返回错误: %v", err)
	}
	byPlatform := map[string]PackResult{}
	for _, res := range results {
		byPlatform[res.Platform] = res
	}
	if !byPlatform["arm"].OK {
		t.Errorf("arm 应当成功，得到 %+v", byPlatform["arm"])
	}
	if byPlatform["mips"].OK {
		t.Errorf("mips 架构目录不存在，不该成功：%+v", byPlatform["mips"])
	}
	if !strings.Contains(byPlatform["mips"].Log, "building/platform/mips") {
		t.Errorf("失败原因应当点出缺少哪个架构目录，得到 %q", byPlatform["mips"].Log)
	}
}

// TestPackRejectsTraversalInManifestFields 是 S2 的回归：appname / version / platform
// 会被拼进产物文件名，而 manifest 是用户（以及导入的 fpk）能随便写的文本。
//
// 以前 `version = ../../../../tmp/pwned` 会被拼成 `<项目根>/demo/demo-../../../../tmp/pwned-x86.fpk`，
// 经 filepath.Join 规整之后写到项目根之外。现在必须在这里就被拒掉，且不留下任何文件。
func TestPackRejectsTraversalInManifestFields(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantWord string
	}{
		{"version 带 ..", "appname=demo\nversion=../../pwned\nplatform=x86\n", "version"},
		{"appname 带 ..", "appname=../evil\nversion=1.0.0\nplatform=x86\n", "appname"},
		{"appname 带分隔符", "appname=a/b\nversion=1.0.0\nplatform=x86\n", "appname"},
		{"platform 带 ..", "appname=demo\nversion=1.0.0\nplatform=../../x\n", "platform"},
		{"platform 为空", "appname=demo\nversion=1.0.0\nplatform=\n", "platform"},
		{"appname 带控制字符", "appname=de\x01mo\nversion=1.0.0\nplatform=x86\n", "appname"},
		{"appname 过长", "appname=" + strings.Repeat("a", 65) + "\nversion=1.0.0\nplatform=x86\n", "appname"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, cfg := newPackManager(t)
			seedProjectWithManifest(t, m.ProjectRoot(), tc.manifest)

			_, err := m.Pack(context.Background(), PackRequest{Name: "demo"})
			if err == nil {
				t.Fatal("畸形的 manifest 字段应当让打包直接失败")
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("错误应当能被判成「用户输入不合法」（HTTP 层据此回 400），得到 %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantWord) {
				t.Errorf("错误里应当点出是哪个字段不对（期望包含 %q）：%v", tc.wantWord, err)
			}

			// 项目根之外（整个数据目录里，除了项目目录本身）不能出现任何 fpk。
			projectDir := filepath.Join(m.ProjectRoot(), "demo")
			_ = filepath.WalkDir(cfg.DataDir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".fpk") {
					return nil
				}
				if strings.HasPrefix(path, projectDir+string(filepath.Separator)) {
					return nil
				}
				t.Errorf("越界写出了产物：%s", path)
				return nil
			})
		})
	}
}

// TestPackRejectsServerDirAtProjectRoot：serverDir 填 "." 时，prepareStage 的
// 「先删干净」会把整个暂存项目（含 manifest）删掉，后面只会报一句莫名其妙的
// 「没有 manifest」，所以要在入口就拦住。
func TestPackRejectsServerDirAtProjectRoot(t *testing.T) {
	for _, serverDir := range []string{".", "./", "app/.."} {
		t.Run(serverDir, func(t *testing.T) {
			m, _ := newPackManager(t)
			seedProject(t, m.ProjectRoot())
			_, err := m.Pack(context.Background(), PackRequest{Name: "demo", ServerDir: serverDir})
			if err == nil || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("serverDir=%q 应当被拒绝，得到 %v", serverDir, err)
			}
			if !strings.Contains(err.Error(), "server 目录") {
				t.Errorf("错误应当点出 server 目录：%v", err)
			}
		})
	}
}

// TestPackAllowsVersionWithDashesAndDots：别把合法的版本号一起挡掉。
func TestPackAllowsVersionWithDashesAndDots(t *testing.T) {
	m, _ := newPackManager(t)
	seedProjectWithManifest(t, m.ProjectRoot(), "appname=demo\nversion=1.0.0-beta.2+build7\nplatform=x86\n")
	t.Setenv("FPKSTUDIO_TEST_STAGE", filepath.Join(t.TempDir(), "stage.txt"))

	results, err := m.Pack(context.Background(), PackRequest{Name: "demo"})
	if err != nil {
		t.Fatalf("合法版本号不该被拒：%v", err)
	}
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("打包失败：%+v", results)
	}
	if results[0].File != "demo-1.0.0-beta.2+build7-x86.fpk" {
		t.Errorf("产物名 = %q", results[0].File)
	}
}
