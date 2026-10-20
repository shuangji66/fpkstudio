package fsx

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// timeAfter 给「会不会挂住」这类断言用：默认 5 秒，够慢机器上的 tar 启动。
func timeAfter() <-chan time.Time { return time.After(5 * time.Second) }

// 解压这条路上「沙箱帮不上忙」的那几处（外部命令吃绝对路径）以及会被留下的特殊文件，
// 都在这里钉住。

// TestExtractRejectsSymlinkArchive：归档本身是符号链接时不能解压。
//
// 交给 tar / gzip 的是 x.Abs(rel)，os.Root 拦不住——一个指向 Root 之外的链接就能让
// 解压把外部文件的内容读进项目里（实测过：ReadFile 报 escapes，Extract 却成功）。
func TestExtractRejectsSymlinkArchive(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt.gz")
	f, err := os.Create(secret)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	_, _ = gz.Write([]byte("TOP-SECRET\n"))
	_ = gz.Close()
	_ = f.Close()

	root := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(root, "leak.gz")); err != nil {
		t.Fatal(err)
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if _, err := x.Extract(context.Background(), "leak.gz"); err == nil {
		t.Fatal("符号链接归档应当被拒绝")
	}
	if _, err := os.Stat(filepath.Join(root, "leak")); err == nil {
		t.Fatal("外部文件的内容被解压进项目了")
	}
}

// TestExtractRejectsFifoArchive：归档本身是 FIFO 时不能解压（否则 open 会永久阻塞）。
func TestExtractRejectsFifoArchive(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe.tar"), 0o644); err != nil {
		t.Skipf("这个文件系统建不了 FIFO: %v", err)
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	done := make(chan error, 1)
	go func() {
		_, err := x.Extract(context.Background(), "pipe.tar")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO 归档应当被拒绝")
		}
	case <-timeAfter():
		t.Fatal("Extract 在 FIFO 归档上挂住了")
	}
}

// TestExtractDropsSpecialMembers：归档里的 FIFO 成员不能留在项目里。
//
// 留着它的表现是「之后每次读它 / 解压它都永久阻塞」：一个请求占一个 goroutine 加一个
// tar 子进程，客户端断开也不结束。tar 造 FIFO 不需要 root。
func TestExtractDropsSpecialMembers(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "pipe", Typeflag: tar.TypeFifo, Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	body := "hello\n"
	if err := tw.WriteHeader(&tar.Header{Name: "ok.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	if err := os.WriteFile(filepath.Join(root, "bundle.tar"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	res, err := x.Extract(context.Background(), "bundle.tar")
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if res.Specials != 1 {
		t.Errorf("Specials = %d，期望 1（要把丢掉的特殊文件数报给界面）", res.Specials)
	}
	if _, err := os.Lstat(filepath.Join(root, "pipe")); err == nil {
		t.Error("FIFO 成员被留在了项目里")
	}
	if got, err := x.ReadFile("ok.txt"); err != nil || string(got) != body {
		t.Errorf("普通文件没解出来: %q, %v", got, err)
	}
}

// TestReadFileRejectsFifo：即便项目里已经有 FIFO（手工建的、旧版本留下的），
// 读它也必须立刻报错，而不是把请求挂在那里。
func TestReadFileRejectsFifo(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0o644); err != nil {
		t.Skipf("这个文件系统建不了 FIFO: %v", err)
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	done := make(chan error, 1)
	go func() {
		_, err := x.ReadFile("pipe")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("读 FIFO 应当报错")
		}
	case <-timeAfter():
		t.Fatal("ReadFile 在 FIFO 上挂住了")
	}
}

// TestZipDirKeepsDotNames：下载整个项目根（base 为 "."）时不能把点开头的名字吃掉一个点。
//
// 以前 `.hidden/a.txt` 会变成 `hidden/a.txt`，和真正的 `hidden/` 撞名，zip 里两个
// 同名条目互相覆盖。
func TestZipDirKeepsDotNames(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{".hidden/a.txt", "hidden/b.txt"} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	var buf bytes.Buffer
	if err := x.ZipDir(".", &buf); err != nil {
		t.Fatalf("ZipDir 失败: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{".hidden/", ".hidden/a.txt", "hidden/", "hidden/b.txt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("zip 里缺少条目 %q：%v", want, names)
		}
	}
}

// TestZipDirKeepsTopLevelDir：下载一个目录时，zip 里要有一层和目录同名的最外层目录。
//
// 少了这一层，「解压到当前目录」会把 manifest、app 直接撒在用户选的地方，和已有文件
// 混在一起——所以这是刻意保留的，别当成冗余条目删掉。空目录也得有这一条：整个包只有
// 它一个成员时才不是一个「没有任何成员的包」（那种包解压工具会直接报错）。
func TestZipDirKeepsTopLevelDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "demo", "app", "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo", "manifest"), []byte("appname=demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	names := func(rel string) []string {
		t.Helper()
		var buf bytes.Buffer
		if err := x.ZipDir(rel, &buf); err != nil {
			t.Fatalf("ZipDir(%s) 失败: %v", rel, err)
		}
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(zr.File))
		for _, f := range zr.File {
			out = append(out, f.Name)
		}
		return out
	}

	got := strings.Join(names("demo"), " ")
	for _, want := range []string{"demo/", "demo/manifest", "demo/app/", "demo/app/server/"} {
		if !strings.Contains(got, want) {
			t.Errorf("zip 里缺少条目 %q：%s", want, got)
		}
	}
	// 摊平的那些条目不允许再出现（`demo/manifest` 里也有个 manifest，所以比完整名字）
	if strings.Contains(got, " manifest") || strings.HasPrefix(got, "manifest") {
		t.Errorf("顶层不该出现没有目录前缀的条目：%s", got)
	}

	if empty := names("empty"); len(empty) != 1 || empty[0] != "empty/" {
		t.Errorf("空目录的 zip 应当只有顶层目录这一条，得到 %v", empty)
	}
}

// TestZipDirExceptSkipsSubtree：项目导出要排除 building/，而且只排除指定子树
// （同名目录出现在别处不能跟着消失）。
func TestZipDirExceptSkipsSubtree(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"demo/building/platform/arm", "demo/app/building"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"demo/manifest", "demo/building/platform/arm/main", "demo/app/building/keep.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	var buf bytes.Buffer
	skip := func(p string) bool { return p == "demo/building" || strings.HasPrefix(p, "demo/building/") }
	if err := x.ZipDirExcept("demo", &buf, skip); err != nil {
		t.Fatalf("ZipDirExcept 失败: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"demo/", "demo/manifest", "demo/app/building/", "demo/app/building/keep.txt"} {
		if !strings.Contains(joined, want) {
			t.Errorf("zip 里缺少条目 %q：%v", want, names)
		}
	}
	for _, gone := range []string{"demo/building", "demo/building/platform", "demo/building/platform/arm/main"} {
		if strings.Contains(joined, gone) {
			t.Errorf("被排除的子树 %q 仍在 zip 里：%v", gone, names)
		}
	}
}

// TestZipDirReportsUnreadableFile：读不了的文件必须让 ZipDir 报错（调用方据此回一句
// 人话，而不是发一个「里面是 0 字节条目」的残包）。
func TestZipDirReportsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 无视模式位，这条测不了")
	}
	root := t.TempDir()
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	// 飞牛存储卷用的是 ACL 模型：模式位为 0 的文件照样读得到。那种环境下这条测不了
	// （也正是「0000 文件读不了」这个前提本身不成立）。
	if f, err := os.Open(secret); err == nil {
		_ = f.Close()
		t.Skip("这个文件系统上模式位不生效（ACL 授权），0000 文件仍然可读")
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	var buf bytes.Buffer
	if err := x.ZipDir(".", &buf); err == nil {
		t.Fatal("读不了的文件应当让 ZipDir 报错")
	}
}

// TestMkdirRootKeepsMode：`Mkdir(".", …, true)` 不能改沙箱根目录的模式。
//
// 项目根可以是用户授权的目录，权限本来更紧；`/api/fs/create {"path":".","dir":true}`
// 以前会把它静默放宽到 0755。
func TestMkdirRootKeepsMode(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	x, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if err := x.Mkdir(".", 0o755, true); err != nil {
		t.Fatalf("Mkdir(\".\") 失败: %v", err)
	}
	fi, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("根目录模式被改成了 %04o，期望保持 0700", fi.Mode().Perm())
	}
}

// TestRelKeepsSpaces：带首尾空格的文件名要能正常寻址（SMB / Windows 上很常见），
// 只有「整串都是空白」才当成根目录。
func TestRelKeepsSpaces(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"x ", "x "},
		{" x", " x"},
		{"   ", "."},
		{"", "."},
		{"..", ""},
		// path.Clean 会先消掉 a/../，所以这不是「含 .. 的路径」而是 b
		{"a/../b", "b"},
	}
	for _, c := range cases {
		got, err := Rel(c.in)
		if c.want == "" {
			if err == nil {
				t.Errorf("Rel(%q) 应当被拒绝，得到 %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Rel(%q) 报错: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Rel(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
