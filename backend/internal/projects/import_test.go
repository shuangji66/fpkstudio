package projects

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"path/filepath"
	"testing"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
)

// 导入的 fpk 里只有打包后的 app/，没有 building/ 工作目录。导入后必须预置
// arm / x86，否则「多架构发布」勾上之后一个架构都没有（前端也会再兜一次）。
func TestImportCreatesDefaultPlatformDirs(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir:     dir,
		ProjectsDir: filepath.Join(dir, "projects"),
		OutputDir:   filepath.Join(dir, "output"),
	}
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

	info, err := m.Import(context.Background(), bytes.NewReader(fakeFPK(t)), "demo.fpk")
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if info.Name != "demo" {
		t.Fatalf("项目名 = %q，期望 demo", info.Name)
	}

	for _, arch := range defaultPlatforms {
		rel := info.Name + "/building/platform/" + arch
		st, err := m.prjRoot().Stat(rel)
		if err != nil {
			t.Errorf("缺少 %s: %v", rel, err)
			continue
		}
		if !st.Dir {
			t.Errorf("%s 不是目录", rel)
		}
	}
}

// fakeFPK 造一个最小的 fpk（gzip tar）：只有 manifest 和一个 app/ 目录，
// 没有 app.tgz——unpackFPK 对这两种形态都认。
func fakeFPK(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	write := func(name string, dir bool, data string) {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}
		if dir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
			hdr.Size = 0
			data = ""
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("写 tar 头失败: %v", err)
		}
		if data != "" {
			if _, err := tw.Write([]byte(data)); err != nil {
				t.Fatalf("写 tar 内容失败: %v", err)
			}
		}
	}

	write("app/", true, "")
	write("manifest", false, "appname               = demo\nversion               = 1.0.0\nplatform              = x86\n")

	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar 失败: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("关闭 gzip 失败: %v", err)
	}
	return buf.Bytes()
}
