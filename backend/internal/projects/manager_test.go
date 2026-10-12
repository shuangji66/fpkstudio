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

// 这一组补的是「打包之外」的项目管理路径：以前 Pack 以外的函数覆盖率是 0，
// 而其中 Delete / Create 都是会动用户文件的。

// TestSettingsRoundTrip：打包偏好读写一圈，nil 的 platforms 要回成空数组
// （nil slice 会被编码成 null，前端拿它展开会直接抛异常）。
func TestSettingsRoundTrip(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.prjRoot().Mkdir("demo", 0o755, true); err != nil {
		t.Fatal(err)
	}

	// 没有 settings.json 时给默认值
	got, err := m.GetSettings("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerDir != DefaultServerDir || !got.CopyToOutput || got.Platforms == nil {
		t.Fatalf("默认设置不对：%+v", got)
	}

	if err := m.SaveSettings("demo", Settings{
		MultiPlatform: true,
		Platforms:     []string{"arm"},
		ServerDir:     "app/bin",
		CopyToOutput:  false,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = m.GetSettings("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !got.MultiPlatform || got.ServerDir != "app/bin" || got.CopyToOutput || len(got.Platforms) != 1 {
		t.Fatalf("读回来的设置不对：%+v", got)
	}

	// ServerDir 为空要回落到默认值
	if err := m.SaveSettings("demo", Settings{Platforms: []string{}}); err != nil {
		t.Fatal(err)
	}
	got, _ = m.GetSettings("demo")
	if got.ServerDir != DefaultServerDir {
		t.Errorf("空 ServerDir 应当回落到 %s，得到 %q", DefaultServerDir, got.ServerDir)
	}
	if got.Platforms == nil {
		t.Error("Platforms 不能是 nil")
	}

	// 坏 JSON：不报错，按默认值处理（不能让一个坏文件把界面卡住）
	if err := m.prjRoot().WriteFile("demo/"+settingsFile, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err = m.GetSettings("demo"); err != nil || got.ServerDir != DefaultServerDir {
		t.Errorf("坏 settings.json 应当回落默认值，得到 %+v / %v", got, err)
	}
}

// TestPlatformsListsOnlyDirs：架构列表只认目录，且按名字排序。
func TestPlatformsListsOnlyDirs(t *testing.T) {
	m, _ := newTestManager(t)
	base := "demo/building/platform"
	for _, d := range []string{"x86", "arm", "loongarch"} {
		if err := m.prjRoot().Mkdir(base+"/"+d, 0o755, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.prjRoot().WriteFile(base+"/not-a-dir.txt", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := m.Platforms("demo")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"arm", "loongarch", "x86"}
	if strings.Join(list, ",") != strings.Join(want, ",") {
		t.Errorf("架构列表 = %v，期望 %v", list, want)
	}

	// 目录不存在时给空列表，而不是报错（新项目还没建过 building/）
	empty, err := m.Platforms("nope")
	if err != nil || len(empty) != 0 {
		t.Errorf("不存在的项目应当给空列表，得到 %v / %v", empty, err)
	}
}

// TestDeleteProject：删掉之后要真的不在，再删一次报 ErrNotFound。
func TestDeleteProject(t *testing.T) {
	m, _ := newTestManager(t)
	seedProject(t, m.ProjectRoot())

	if err := m.Delete("demo"); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := os.Stat(filepath.Join(m.ProjectRoot(), "demo")); err == nil {
		t.Error("项目目录还在")
	}
	if err := m.Delete("demo"); !errors.Is(err, ErrNotFound) {
		t.Errorf("再删一次应当报 ErrNotFound，得到 %v", err)
	}
	if err := m.Delete("../evil"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("非法项目名应当报 ErrInvalidName，得到 %v", err)
	}
}

// fakeFnpackCreate 造一个会（或不会）真的建出骨架的假 fnpack。
func fakeFnpackCreate(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fnpack")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func managerWithFnpack(t *testing.T, bin string) *Manager {
	t.Helper()
	data := t.TempDir()
	cfg := &config.Config{
		DataDir:     data,
		ProjectsDir: filepath.Join(data, "projects"),
		OutputDir:   filepath.Join(data, "output"),
		TmpDir:      filepath.Join(data, "tmp"),
		FnpackBin:   bin,
	}
	log, err := logging.New("", "error", 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	m, err := New(cfg, log, fnpack.New(cfg, log))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

// TestCreateProject：fnpack create 之后要补齐 building/platform/<arch>，
// 并清掉模板里的 .DS_Store（它会让后续 build 直接失败）。
func TestCreateProject(t *testing.T) {
	bin := fakeFnpackCreate(t, `#!/bin/sh
if [ "$1" = "create" ]; then
  mkdir -p "$2"
  printf 'appname = %s\nversion = 1.0.0\nplatform = x86\n' "$2" > "$2/manifest"
  touch "$2/.DS_Store"
fi
exit 0
`)
	m := managerWithFnpack(t, bin)

	if err := m.Create(context.Background(), "demo", false, true); err != nil {
		t.Fatalf("创建项目失败: %v", err)
	}
	for _, rel := range []string{"demo/manifest", "demo/building/platform/arm", "demo/building/platform/x86"} {
		if _, err := m.prjRoot().Stat(rel); err != nil {
			t.Errorf("缺少 %s: %v", rel, err)
		}
	}
	if _, err := m.prjRoot().Stat("demo/.DS_Store"); err == nil {
		t.Error("模板里的 .DS_Store 没被清掉")
	}
	// 重名要拒绝
	if err := m.Create(context.Background(), "demo", false, true); !errors.Is(err, ErrExists) {
		t.Errorf("重名创建应当报 ErrExists，得到 %v", err)
	}
}

// TestCreateFailureUsesFnpackOutput：fnpack create 失败时退出码还是 0，
// 判据必须是「目录有没有真的落地」，并把 fnpack 自己那句话带出来。
func TestCreateFailureUsesFnpackOutput(t *testing.T) {
	bin := fakeFnpackCreate(t, `#!/bin/sh
echo "Create application name ab error : appname ilegal,app name (ab) length must be between 3 and 32"
exit 0
`)
	m := managerWithFnpack(t, bin)

	err := m.Create(context.Background(), "ab", false, true)
	if err == nil {
		t.Fatal("应当报失败")
	}
	if !strings.Contains(err.Error(), "appname ilegal") {
		t.Errorf("错误里应当带出 fnpack 的原话：%v", err)
	}
}

// TestCreateTrimsName：名字带首尾空格时，判重与建目录必须用同一个名字。
func TestCreateTrimsName(t *testing.T) {
	bin := fakeFnpackCreate(t, `#!/bin/sh
if [ "$1" = "create" ]; then
  mkdir -p "$2"
  printf 'appname = %s\nversion = 1.0.0\nplatform = x86\n' "$2" > "$2/manifest"
fi
exit 0
`)
	m := managerWithFnpack(t, bin)

	if err := m.Create(context.Background(), "  demo  ", false, true); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if _, err := m.prjRoot().Stat("demo/manifest"); err != nil {
		t.Errorf("项目应当落在 demo 下: %v", err)
	}
}
