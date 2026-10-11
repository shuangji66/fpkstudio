package fsx

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 解压走的是系统 tar / gzip / bzip2 / xz / zstd，没有对应工具就跳过
// （不把「环境里没装」判成代码错）。
func requireTools(t *testing.T, bins ...string) {
	t.Helper()
	for _, bin := range bins {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("环境里没有 %s，跳过解压测试", bin)
		}
	}
}

func TestArchiveKindOf(t *testing.T) {
	cases := map[string]string{
		"app.tar":      "tar",
		"app.TAR":      "tar",
		"app.tar.gz":   "tar.gz",
		"app.TGZ":      "tar.gz",
		"app.tar.bz2":  "tar.bz2",
		"app.tbz2":     "tar.bz2",
		"app.tbz":      "tar.bz2",
		"app.tar.xz":   "tar.xz",
		"app.txz":      "tar.xz",
		"app.tar.zst":  "tar.zst",
		"app.tzst":     "tar.zst",
		"data.gz":      "gz",
		"data.bz2":     "bz2",
		"data.xz":      "xz",
		"data.zst":     "zst",
		"app.zip":      "zip",
		"app.ZIP":      "zip",
		"manifest":     "",
		"app.7z":       "",
		"app.rar":      "",
		"app.tar.Z":    "",
		"mytar.gz.txt": "",
	}
	for name, want := range cases {
		got, ok := ArchiveKindOf(name)
		if want == "" {
			if ok {
				t.Errorf("ArchiveKindOf(%q) 应当不认识，却返回 %q", name, got)
			}
			continue
		}
		if !ok || string(got) != want {
			t.Errorf("ArchiveKindOf(%q) = %q/%v，期望 %q", name, got, ok, want)
		}
	}
}

func TestCheckMembersRejectsEscape(t *testing.T) {
	bad := []string{
		"/etc/passwd",
		"../evil",
		"a/../../evil",
		"a/..",
		"..\n",
	}
	for _, name := range bad {
		if err := checkMembers([]byte(name + "\n")); err == nil {
			t.Errorf("成员 %q 应当被拒绝", name)
		}
	}
	good := "app/\napp/server\nmanifest\n..hidden/a..b\n"
	if err := checkMembers([]byte(good)); err != nil {
		t.Errorf("正常成员被拒: %v", err)
	}
}

// makeTar 用标准库拼一个 tar：系统 tar 不肯产出 ../ 这种成员，只能自己造。
func makeTar(t *testing.T, members map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range members {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(name, "/") {
			hdr = &tar.Header{Name: name, Mode: 0o755, Typeflag: tar.TypeDir}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "evil.tar")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractRejectsTraversal(t *testing.T) {
	requireTools(t, TarBin)
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	// 归档想写到项目根目录外面去：必须在解压之前就被挡下来。
	src := makeTar(t, map[string]string{"../evil.txt": "pwned"})
	if err := copyInto(t, src, filepath.Join(root, "evil.tar")); err != nil {
		t.Fatal(err)
	}
	_, err = x.Extract("evil.tar")
	if err == nil {
		t.Fatal("含 ../ 成员的归档应当被拒绝")
	}
	// 必须是我们自己的成员名校验挡下来的：tar 对越界名字只给警告，退出码还是 0，
	// 靠它挡不住。这条断言就是为了钉住「拦截发生在解压之前」。
	if !strings.Contains(err.Error(), "跳出目标目录") {
		t.Fatalf("拒绝原因不对（说明不是成员名校验挡的）: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.txt")); err == nil {
		t.Fatal("越界文件被写出来了")
	}
}

func TestExtractTarSkipsExisting(t *testing.T) {
	requireTools(t, TarBin)
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	// 造一个 tar：app/new.txt + app/keep.txt，其中 keep.txt 已经在项目里了。
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, body := range map[string]string{"app/new.txt": "new", "app/keep.txt": "from-archive"} {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := copyInto(t, writeFile(t, "app.tar", buf.Bytes()), filepath.Join(root, "app.tar")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := x.Extract("app.tar")
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if res.Files != 1 || res.Skipped != 1 {
		t.Errorf("Files/Skipped = %d/%d，期望 1/1", res.Files, res.Skipped)
	}
	if res.Dir != "." || res.Kind != "tar" {
		t.Errorf("Dir/Kind = %q/%q，期望 ./tar", res.Dir, res.Kind)
	}

	// 已有的文件不能被覆盖，新的要真的写进去。
	if got, _ := os.ReadFile(filepath.Join(root, "app", "keep.txt")); string(got) != "mine" {
		t.Errorf("已存在的文件被覆盖了: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "app", "new.txt")); string(got) != "new" {
		t.Errorf("新文件没解出来: %q", got)
	}
	// 暂存目录必须清干净。
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".fpkstudio-extract-") {
			t.Errorf("暂存目录没清掉: %s", e.Name())
		}
	}
}

func TestExtractTarGzIntoSubdir(t *testing.T) {
	requireTools(t, TarBin)
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	dir := t.TempDir()
	payload := filepath.Join(dir, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "sub", "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 用系统 tar 造一个 .tar.gz（和用户上传的形态一致）。
	if out, err := exec.Command(TarBin, "-czf", filepath.Join(dir, "bundle.tar.gz"), "-C", payload, "sub").CombinedOutput(); err != nil {
		t.Fatalf("造包失败: %v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyInto(t, filepath.Join(dir, "bundle.tar.gz"), filepath.Join(root, "pkg", "bundle.tar.gz")); err != nil {
		t.Fatal(err)
	}

	res, err := x.Extract("pkg/bundle.tar.gz")
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if res.Dir != "pkg" || res.Kind != "tar.gz" {
		t.Errorf("Dir/Kind = %q/%q，期望 pkg/tar.gz", res.Dir, res.Kind)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "pkg", "sub", "a.txt")); string(got) != "hi" {
		t.Errorf("文件没解到归档所在目录: %q", got)
	}
}

func TestExtractGz(t *testing.T) {
	requireTools(t, GzipBin)
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	dir := t.TempDir()
	plain := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(plain, []byte("hello gz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(GzipBin, "-c", plain).Output(); err != nil {
		t.Fatalf("压缩失败: %v: %s", err, out)
	} else if err := os.WriteFile(filepath.Join(root, "notes.txt.gz"), out, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := x.Extract("notes.txt.gz")
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if res.Kind != "gz" || res.Files != 1 {
		t.Errorf("Kind/Files = %q/%d，期望 gz/1", res.Kind, res.Files)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(got) != "hello gz" {
		t.Errorf("内容不对: %q", got)
	}
}

func TestExtractRejectsBadTarget(t *testing.T) {
	requireTools(t, TarBin)
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if _, err := x.Extract("."); err == nil {
		t.Error("根目录应当被拒绝")
	}
	if err := os.MkdirAll(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Extract("adir"); err == nil {
		t.Error("目录应当被拒绝")
	}
	if err := os.WriteFile(filepath.Join(root, "manifest"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Extract("manifest"); err == nil {
		t.Error("非压缩包应当被拒绝")
	}
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func copyInto(t *testing.T, src, dst string) error {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// 各种 tar 压缩变体：命令里不带压缩参数，靠 tar 自己按魔数认。
func TestExtractTarCompressionVariants(t *testing.T) {
	requireTools(t, TarBin)
	cases := []struct {
		name string
		flag string
	}{
		{"plain.tar", ""},
		{"gzip.tar.gz", "-z"},
		{"bzip2.tar.bz2", "-j"},
		{"xz.tar.xz", "-J"},
		{"zstd.tar.zst", "--zstd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			x, err := Open(root, true)
			if err != nil {
				t.Fatal(err)
			}
			defer x.Close()

			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "pkg", "a.txt"), []byte("hi"), 0o644); err != nil {
				t.Fatal(err)
			}
			args := []string{"-cf", filepath.Join(dir, c.name), "-C", dir, "pkg"}
			if c.flag != "" {
				args = append([]string{c.flag}, args...)
			}
			if out, err := exec.Command(TarBin, args...).CombinedOutput(); err != nil {
				t.Fatalf("造包失败: %v: %s", err, out)
			}
			if err := copyInto(t, filepath.Join(dir, c.name), filepath.Join(root, c.name)); err != nil {
				t.Fatal(err)
			}

			res, err := x.Extract(c.name)
			if err != nil {
				t.Fatalf("解压失败: %v", err)
			}
			if res.Files != 1 {
				t.Errorf("Files = %d，期望 1", res.Files)
			}
			if got, _ := os.ReadFile(filepath.Join(root, "pkg", "a.txt")); string(got) != "hi" {
				t.Errorf("内容不对: %q", got)
			}
		})
	}
}

// 单个文件的 bz2 / xz / zst，和 gz 一样解出同名文件。
func TestExtractSingleFileCompressors(t *testing.T) {
	cases := []struct {
		suffix string
		bin    string
		args   []string
	}{
		{".gz", GzipBin, []string{"-c"}},
		{".bz2", Bzip2Bin, []string{"-c"}},
		{".xz", XzBin, []string{"-c"}},
		{".zst", ZstdBin, []string{"-q", "-c"}},
	}
	for _, c := range cases {
		t.Run(c.suffix, func(t *testing.T) {
			requireTools(t, c.bin)
			root := t.TempDir()
			x, err := Open(root, true)
			if err != nil {
				t.Fatal(err)
			}
			defer x.Close()

			dir := t.TempDir()
			plain := filepath.Join(dir, "notes.txt")
			if err := os.WriteFile(plain, []byte("hello"), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(c.bin, append(c.args, plain)...).Output()
			if err != nil {
				t.Fatalf("压缩失败: %v", err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes.txt"+c.suffix), out, 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := x.Extract("notes.txt" + c.suffix); err != nil {
				t.Fatalf("解压失败: %v", err)
			}
			if got, _ := os.ReadFile(filepath.Join(root, "notes.txt")); string(got) != "hello" {
				t.Errorf("内容不对: %q", got)
			}
		})
	}
}

// zip 走标准库：正常解压、同名跳过、越界拒绝。
func TestExtractZip(t *testing.T) {
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if err := copyInto(t, makeZip(t, map[string]string{
		"app/new.txt":  "new",
		"app/keep.txt": "from-zip",
		"top.txt":      "top",
	}), filepath.Join(root, "bundle.zip")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "keep.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := x.Extract("bundle.zip")
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	if res.Kind != "zip" {
		t.Errorf("Kind = %q，期望 zip", res.Kind)
	}
	if res.Files != 2 || res.Skipped != 1 {
		t.Errorf("Files/Skipped = %d/%d，期望 2/1", res.Files, res.Skipped)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "app", "keep.txt")); string(got) != "mine" {
		t.Errorf("已存在的文件被覆盖了: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "app", "new.txt")); string(got) != "new" {
		t.Errorf("新文件没解出来: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "top.txt")); string(got) != "top" {
		t.Errorf("顶层文件没解出来: %q", got)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	x, err := Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()

	if err := copyInto(t, makeZip(t, map[string]string{"../evil.txt": "pwned"}), filepath.Join(root, "evil.zip")); err != nil {
		t.Fatal(err)
	}
	_, err = x.Extract("evil.zip")
	if err == nil {
		t.Fatal("含 ../ 成员的 zip 应当被拒绝")
	}
	if !strings.Contains(err.Error(), "跳出目标目录") {
		t.Fatalf("拒绝原因不对: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.txt")); err == nil {
		t.Fatal("越界文件被写出来了")
	}
}

// 反斜杠分隔的成员名同样要挡住（Windows 打的包会用 \）。
func TestCheckMemberBackslash(t *testing.T) {
	for _, name := range []string{"..\\evil", "a\\..\\..\\evil", "\\etc\\passwd"} {
		if err := checkMember(name); err == nil {
			t.Errorf("成员 %q 应当被拒绝", name)
		}
	}
	if err := checkMember("a\\b\\c"); err != nil {
		t.Errorf("正常成员被拒: %v", err)
	}
}

// makeZip 用标准库拼一个 zip。
func makeZip(t *testing.T, members map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "bundle.zip")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
