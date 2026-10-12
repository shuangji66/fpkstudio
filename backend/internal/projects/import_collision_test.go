package projects

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fpkstudio/internal/fsx"
)

// fakeFPKVersion 造一个最小 fpk（gzip tar），manifest 里的 version 由调用方给，
// 用来区分「同一个 appname 的多个包」。
func fakeFPKVersion(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name, data string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatalf("写 tar 头失败: %v", err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatalf("写 tar 内容失败: %v", err)
		}
	}
	write("manifest", "appname               = demo\nversion               = "+version+"\nplatform              = x86\n")
	write("app/hello.txt", "hello "+version+"\n")
	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar 失败: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("关闭 gzip 失败: %v", err)
	}
	return buf.Bytes()
}

// TestImportSameNameCollisionKeepsBoth 是 M4 的回归。
//
// 以前重名时只把名字改成 `<name>-<unix 秒>` 就完事、**不再复查**：同一秒里导入第二个
// 同 appname 的包时那个名字已经存在，moveIntoProjects 的 rename 报 ENOTEMPTY、退回
// fsx.CopyTree 合并 —— 第三个包把第二个项目的内容静默覆盖掉，界面还报「导入成功」。
func TestImportSameNameCollisionKeepsBoth(t *testing.T) {
	m, _ := newTestManager(t)
	ctx := context.Background()

	versions := []string{"1.0.0", "2.0.0", "3.0.0"}
	names := make([]string, 0, len(versions))
	for i, v := range versions {
		info, err := m.Import(ctx, bytes.NewReader(fakeFPKVersion(t, v)), "demo.fpk")
		if err != nil {
			t.Fatalf("第 %d 次导入失败: %v", i+1, err)
		}
		names = append(names, info.Name)
	}

	// 三次导入必须是三个不同的项目目录。
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Fatalf("导入返回了重复的项目名 %q（%v）", name, names)
		}
		seen[name] = true
	}

	entries, err := os.ReadDir(m.ProjectRoot())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(versions) {
		t.Errorf("项目目录数 = %d，期望 %d（%v）", len(entries), len(versions), names)
	}

	// 每个项目里必须还是它自己那一份内容：被覆盖的表现是「三个目录里都是 3.0.0」。
	for i, name := range names {
		data, err := m.prjRoot().ReadFile(name + "/manifest")
		if err != nil {
			t.Fatalf("读 %s/manifest 失败: %v", name, err)
		}
		if !strings.Contains(string(data), "version               = "+versions[i]) {
			t.Errorf("项目 %s 的 manifest 不是它自己那份（期望 version = %s）：\n%s", name, versions[i], data)
		}
		body, err := m.prjRoot().ReadFile(name + "/app/hello.txt")
		if err != nil {
			t.Fatalf("读 %s/app/hello.txt 失败: %v", name, err)
		}
		if !strings.Contains(string(body), versions[i]) {
			t.Errorf("项目 %s 的 app/hello.txt 被别的包覆盖了：%q", name, body)
		}
	}
}

// TestCopyTreeRefusesExistingTarget：CopyTree 是「导入落地」的最后一道，
// 它必须和 Root.Copy 一样拒绝已存在的目标，不能静默合并。
func TestCopyTreeRefusesExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	for _, d := range []string{src, dst} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("src"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "a.txt"), []byte("用户的文件"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := fsx.CopyTree(src, dst)
	if err == nil {
		t.Fatal("目标已存在时 CopyTree 应当拒绝")
	}
	body, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(body) != "用户的文件" {
		t.Errorf("目标文件被覆盖了：%q", body)
	}
}

// TestImportPathRejectsOversizedFile：走飞牛选择器导入时没有请求体可卡，
// 所以按文件大小判一次（和上传那条路的 1G 对齐）。
func TestImportPathRejectsOversizedFile(t *testing.T) {
	m, _ := newTestManager(t)
	dir := t.TempDir()
	big := filepath.Join(dir, "big.fpk")
	if err := os.WriteFile(big, []byte("xxxx"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := maxImportFileBytes
	maxImportFileBytes = 1
	t.Cleanup(func() { maxImportFileBytes = old })

	_, err := m.ImportPath(context.Background(), big)
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("超限的包应当被拒（ErrInvalidInput），得到 %v", err)
	}
	if !strings.Contains(err.Error(), "超过上限") {
		t.Errorf("错误里应当说清是超限：%v", err)
	}
}
