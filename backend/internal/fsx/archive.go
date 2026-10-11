package fsx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// 解压：tar 系列交给系统的 tar，单个文件的 .gz/.bz2/.xz/.zst 交给对应的解压工具，
// zip 用标准库。
//
// 用外部命令而不是标准库的 archive/tar，是因为用户明确要求「调用系统压缩工具」：
// 行为跟用户在终端里敲 tar 一致（GNU 的各种扩展、长文件名、稀疏文件都由 tar 自己
// 处理）。zip 反过来用 archive/zip：unzip / 7z 不是每个系统都有，而 zip 恰恰是最
// 常见的格式，标准库能省掉一个「目标机器上没装」的坑。
//
// tar 不带压缩参数：GNU tar 会按魔数自己认出 gz / bz2 / xz / zst，一个命令覆盖
// 全部 tar 变体，也省得我们把后缀和压缩算法绑死。
//
// 安全上做三件事：
//  1. tar 自己没有「不许写到目标目录之外」的开关，所以解压前先用 `tar -t` 列一遍
//     成员名，含绝对路径或 `..` 段的归档直接拒绝（成员名是 C 风格转义输出的，
//     换行不会把一行拆成两行）；zip 的成员名同样逐个校验。
//  2. 先解到目标目录里的一个暂存目录，再逐个搬进来。暂存目录由这一次请求独占，
//     归档里的符号链接够不到它外面；搬迁走 os.Root 的 rename，同样出不去。
//  3. 所有写入都走 os.Root，不用拼绝对路径的 os.OpenFile。
//
// 同名冲突一律**跳过**，不覆盖：解压最常见的情形是往一个已经存在的目录上补内容
// （比如把 app.tgz 解到已经有 app/ 的项目里），静默覆盖用户的文件是不可逆的。
// 跳过的数量会返回给界面，让用户知道哪些没动。

// 解压用的外部命令。
//
// 默认写死 /usr/bin：应用以普通用户跑在飞牛上，服务进程的 PATH 不保证有 /usr/bin
// （应用商店起的进程常常只有很窄的一段），而这些工具在飞牛上就在那儿。文件不在
// （别的发行版可能装在别处）才回落到 PATH 查找，不给它们加环境变量——多一个配置项
// 就多一处「没配好就解压不了」。
var (
	TarBin   = "/usr/bin/tar"
	GzipBin  = "/usr/bin/gzip"
	Bzip2Bin = "/usr/bin/bzip2"
	XzBin    = "/usr/bin/xz"
	ZstdBin  = "/usr/bin/zstd"
)

// binPath 解析要用的外部命令。
//
// 两边都找不到时给一句人话：exec 自己报的是「executable file not found in $PATH」，
// 用户看到只会以为解压坏了。
func binPath(absolute string) (string, error) {
	if _, err := os.Stat(absolute); err == nil {
		return absolute, nil
	}
	name := path.Base(absolute)
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("系统里没有 %s，装一个才能解压这种包", name)
}

// ArchiveKind 是能识别的归档类型。
type ArchiveKind string

const (
	// KindTar 是未压缩的 tar。
	KindTar ArchiveKind = "tar"
	// KindTarGz 是 gzip 压过的 tar（.tar.gz / .tgz）。
	KindTarGz ArchiveKind = "tar.gz"
	// KindTarBz2 是 bzip2 压过的 tar（.tar.bz2 / .tbz2 / .tbz）。
	KindTarBz2 ArchiveKind = "tar.bz2"
	// KindTarXz 是 xz 压过的 tar（.tar.xz / .txz）。
	KindTarXz ArchiveKind = "tar.xz"
	// KindTarZst 是 zstd 压过的 tar（.tar.zst / .tzst）。
	KindTarZst ArchiveKind = "tar.zst"
	// KindGz 是单个文件用 gzip 压出来的 .gz（不是 tar）。
	KindGz ArchiveKind = "gz"
	// KindBz2 是单个文件的 .bz2。
	KindBz2 ArchiveKind = "bz2"
	// KindXz 是单个文件的 .xz。
	KindXz ArchiveKind = "xz"
	// KindZst 是单个文件的 .zst。
	KindZst ArchiveKind = "zst"
	// KindZip 是 zip。
	KindZip ArchiveKind = "zip"
)

// isTar 判断是不是 tar 容器（压缩与否都算）。
func (k ArchiveKind) isTar() bool {
	switch k {
	case KindTar, KindTarGz, KindTarBz2, KindTarXz, KindTarZst:
		return true
	}
	return false
}

// 后缀 → 类型。**长的必须排在短的前面**：`.tar.gz` 要排在 `.gz` 之前，否则
// app.tar.gz 会被认成单个文件的 .gz。判据要和前端 utils/format.ts 的 isArchive
// 一致，免得菜单里给了入口、点下去却报「不支持的格式」。
var archiveKinds = []struct {
	suffix string
	kind   ArchiveKind
}{
	{".tar.gz", KindTarGz},
	{".tgz", KindTarGz},
	{".tar.bz2", KindTarBz2},
	{".tbz2", KindTarBz2},
	{".tbz", KindTarBz2},
	{".tar.xz", KindTarXz},
	{".txz", KindTarXz},
	{".tar.zst", KindTarZst},
	{".tzst", KindTarZst},
	{".tar", KindTar},
	{".gz", KindGz},
	{".bz2", KindBz2},
	{".xz", KindXz},
	{".zst", KindZst},
	{".zip", KindZip},
}

// ArchiveKindOf 按文件名判断归档类型；不认识的返回 false（界面据此决定「解压」
// 菜单项出不出现）。
func ArchiveKindOf(name string) (ArchiveKind, bool) {
	lower := strings.ToLower(name)
	for _, entry := range archiveKinds {
		if strings.HasSuffix(lower, entry.suffix) {
			return entry.kind, true
		}
	}
	return "", false
}

// ExtractResult 是一次解压的结果。
type ExtractResult struct {
	// Kind 是识别出来的归档类型。
	Kind string `json:"kind"`
	// Dir 是解压到的目录（相对项目根目录）。
	Dir string `json:"dir"`
	// Files 是本次真正写进去的文件数（不含目录）。
	Files int `json:"files"`
	// Skipped 是因为同名已存在而跳过的文件数。
	Skipped int `json:"skipped"`
}

// Extract 把 rel 指向的归档解压到它所在的目录。
func (x *Root) Extract(rel string) (ExtractResult, error) {
	var res ExtractResult

	rel, err := Rel(rel)
	if err != nil {
		return res, err
	}
	if rel == "." {
		return res, errors.New("请选择一个压缩包")
	}
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return res, err
	}
	if fi.IsDir() {
		return res, errors.New("目标是目录，不能解压")
	}

	base := path.Base(rel)
	kind, ok := ArchiveKindOf(base)
	if !ok {
		return res, fmt.Errorf("不支持的压缩包格式：%s", base)
	}
	res.Kind = string(kind)
	res.Dir = path.Dir(rel)

	staging := path.Join(res.Dir, fmt.Sprintf(".fpkstudio-extract-%d", time.Now().UnixNano()))
	if err := x.r.Mkdir(staging, 0o700); err != nil {
		return res, fmt.Errorf("创建暂存目录失败: %w", err)
	}
	// 无论成败都清掉暂存目录：留下它只会让用户的目录里多一个看不懂的东西。
	defer func() { _ = x.r.RemoveAll(staging) }()

	switch {
	case kind.isTar():
		err = x.untar(x.Abs(rel), staging)
	case kind == KindZip:
		err = x.unzip(rel, staging)
	default:
		err = x.decompress(x.Abs(rel), base, staging, kind)
	}
	if err != nil {
		return res, err
	}

	if err := x.mergeInto(staging, res.Dir, &res); err != nil {
		return res, err
	}
	return res, nil
}

// untar 先校验成员名，再用 tar 解到暂存目录（staging 是相对路径）。
//
// 不带 -z / -j / -J / --zstd：GNU tar 读归档时会按魔数认出压缩格式，一个命令覆盖
// 全部 tar 变体。
func (x *Root) untar(srcAbs, staging string) error {
	out, err := runTar("-tf", srcAbs)
	if err != nil {
		return err
	}
	if err := checkMembers(out); err != nil {
		return err
	}

	if _, err := runTar("-xf", srcAbs, "-C", x.Abs(staging)); err != nil {
		return err
	}
	return nil
}

func runTar(args ...string) ([]byte, error) {
	bin, err := binPath(TarBin)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tar %s 失败: %w: %s", strings.Join(args, " "), err, tailText(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// checkMembers 拒绝会写到目标目录之外的成员。
//
// 只看两件事：绝对路径、以及 `..` 路径段。tar 的输出里名字是转义过的（换行会变成
// `\n` 两个字符），所以按行切是安全的；而任何越界的名字都必然在这两种形态里出现，
// 不会有「转义之后看不出是 ..」的情况。
func checkMembers(listing []byte) error {
	for _, name := range strings.Split(string(listing), "\n") {
		if err := checkMember(strings.TrimSuffix(name, "\r")); err != nil {
			return err
		}
	}
	return nil
}

// checkMember 校验单个成员名：不许绝对路径、不许 `..` 段。
//
// 先把反斜杠归一成斜杠：zip（Windows 打的）和某些 tar 会用 `\` 当分隔符，
// 只按 "/" 切的话 `..\evil` 就绕过去了。
func checkMember(name string) error {
	name = strings.ReplaceAll(name, "\\", "/")
	if name == "" {
		return nil
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("归档里有绝对路径 %q，已拒绝解压", name)
	}
	for _, seg := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if seg == ".." {
			return fmt.Errorf("归档里有跳出目标目录的路径 %q，已拒绝解压", name)
		}
	}
	return nil
}

// 单个文件压缩包 → 解压命令与它对应的后缀。
var singleTools = map[ArchiveKind]struct {
	bin    string
	suffix string
}{
	KindGz:  {GzipBin, ".gz"},
	KindBz2: {Bzip2Bin, ".bz2"},
	KindXz:  {XzBin, ".xz"},
	KindZst: {ZstdBin, ".zst"},
}

// decompress 解压「不是 tar、只是单个文件被压过」的归档：解出来写到暂存目录里的
// 同名文件（去掉压缩后缀）。
func (x *Root) decompress(srcAbs, base, staging string, kind ArchiveKind) error {
	tool, ok := singleTools[kind]
	if !ok {
		return fmt.Errorf("不支持解压这种格式：%s", base)
	}
	bin, err := binPath(tool.bin)
	if err != nil {
		return err
	}

	name := strings.TrimSuffix(base, path.Ext(base))
	if name == "" {
		name = base + ".out"
	}
	f, err := x.r.OpenFile(path.Join(staging, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("创建 %s 失败: %w", name, err)
	}
	defer f.Close()

	cmd := exec.Command(bin, "-dc", srcAbs)
	cmd.Stdout = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("解压 %s 失败: %w: %s", base, err, tailText(stderr.String()))
	}
	return nil
}

// unzip 用标准库解 zip 到暂存目录。
//
// 走标准库而不是 unzip / 7z：它们不是每个系统都有，而 zip 恰恰是最常见的格式。
// 成员名照 tar 那条路一样先全量校验（zip 的 Name 同样能带 `..` 或绝对路径），
// 写入全部走 os.Root。
func (x *Root) unzip(rel, staging string) error {
	f, err := x.r.Open(rel)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return fmt.Errorf("不是有效的 zip：%w", err)
	}

	// 先全量校验再动手：一半解出来、一半被拒会留下一个说不清状态的目录。
	for _, entry := range zr.File {
		if err := checkMember(entry.Name); err != nil {
			return err
		}
	}
	for _, entry := range zr.File {
		if err := x.unzipEntry(entry, staging); err != nil {
			return err
		}
	}
	return nil
}

func (x *Root) unzipEntry(entry *zip.File, staging string) error {
	// Windows 打的包可能用反斜杠；统一成正斜杠再进沙箱。
	name := strings.ReplaceAll(entry.Name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	if name == "" {
		return nil
	}
	rel := path.Join(staging, name)

	if entry.FileInfo().IsDir() {
		return x.r.MkdirAll(rel, 0o755)
	}
	if parent := path.Dir(rel); parent != "." {
		if err := x.r.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}

	src, err := entry.Open()
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", name, err)
	}
	defer src.Close()

	// 符号链接在 zip 里存成「内容是目标路径的普通文件」，按原样还原。
	if entry.Mode()&os.ModeSymlink != 0 {
		target, err := io.ReadAll(io.LimitReader(src, 4096))
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", name, err)
		}
		_ = x.r.Remove(rel)
		return x.r.Symlink(string(target), rel)
	}

	dst, err := x.r.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("创建 %s 失败: %w", name, err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", name, err)
	}
	return nil
}

// mergeInto 把暂存目录里的内容搬进 dest，同名的一律跳过。
func (x *Root) mergeInto(staging, dest string, res *ExtractResult) error {
	entries, err := x.List(staging)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := x.mergeEntry(path.Join(staging, e.Name), path.Join(dest, e.Name), res); err != nil {
			return err
		}
	}
	return nil
}

func (x *Root) mergeEntry(src, dst string, res *ExtractResult) error {
	sfi, err := x.r.Lstat(src)
	if err != nil {
		return err
	}
	dfi, err := x.r.Lstat(dst)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// 目标没有同名项：直接 rename 就位（同一个文件系统，不搬内容）。
		if err := x.r.Rename(src, dst); err != nil {
			return err
		}
		x.repairPerms(dst)
		res.Files += x.countFiles(dst)
		return nil
	case err != nil:
		return err
	}

	// 两边都是目录：往已有的目录里合并（这是「解压到项目目录」最常见的情形）。
	if sfi.IsDir() && dfi.IsDir() {
		children, err := x.List(src)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := x.mergeEntry(path.Join(src, c.Name), path.Join(dst, c.Name), res); err != nil {
				return err
			}
		}
		return nil
	}

	// 其余情况（同名文件、文件对目录、目录对文件）：跳过，绝不覆盖用户已有的东西。
	res.Skipped += x.countFiles(src)
	return nil
}

// countFiles 数一个条目下面有几个文件（不含目录；它自己是文件就算 1）。
func (x *Root) countFiles(rel string) int {
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return 0
	}
	if !fi.IsDir() {
		return 1
	}
	n := 0
	_ = x.Walk(rel, func(_ string, isDir bool, _ string) error {
		if !isDir {
			n++
		}
		return nil
	})
	return n
}

// repairPerms 修一遍刚解压出来的模式位。
//
// 跟 RepairModes 同一个理由：飞牛的存储卷上新建文件的 POSIX 模式位是 0，访问权由
// ACL 提供——本地看着能读，打进 fpk 之后在目标机器上就可能不可用。只在模式位确实
// 为空时补默认值，归档里本来带着的执行位之类的信息不动。
func (x *Root) repairPerms(rel string) {
	_ = x.Walk(rel, func(cur string, isDir bool, _ string) error {
		fi, err := x.r.Lstat(cur)
		if err != nil {
			return nil
		}
		perm := fi.Mode().Perm()
		if isDir {
			if perm&0o700 != 0o700 {
				_ = x.r.Chmod(cur, perm|0o700)
			}
			return nil
		}
		if perm&0o444 == 0 {
			_ = x.r.Chmod(cur, 0o644)
		}
		return nil
	})
}

// tailText 取命令输出的最后一段，避免把几 MB 的错误日志原样塞进接口响应。
func tailText(s string) string {
	s = strings.TrimSpace(s)
	const max = 500
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}
