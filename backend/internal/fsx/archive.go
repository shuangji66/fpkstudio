package fsx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// 解压：tar 系列交给系统的 tar，单个文件的 .gz/.bz2/.xz/.zst 交给对应的解压工具，
// zip 交给系统的 unzip。**全部走外部命令**，不用标准库的 archive/tar 与 archive/zip。
//
// 理由和用户的要求一致：行为跟用户在终端里敲一遍一样（GNU tar 的各种扩展、长文件名、
// 稀疏文件；Info-ZIP 的属性位、加密包、跨平台名字），也省得我们自己维护一份
// 「归档格式的边界情况」——这类代码出错的代价是写坏用户的目录。
//
// tar 不带压缩参数：GNU tar 会按魔数自己认出 gz / bz2 / xz / zst，一个命令覆盖
// 全部 tar 变体，也省得我们把后缀和压缩算法绑死。
//
// 安全上做四件事：
//  1. 解压前先列一遍成员名（tar 用 `tar -t`、zip 用 `unzip -Z1`），含绝对路径或
//     `..` 段的归档直接拒绝。**不能只看退出码**：tar 对越界名字只给警告、退出码还是
//     0；Info-ZIP 更省事——它把越界名字截断成一个安全的名字照写不误，退出码给 1。
//     （tar 的输出是 C 风格转义的，换行不会把一行拆成两行；zip 的成员名直接输出，
//     但拆错了只会多校验几行垃圾，判据仍然是「出现越界段就拒绝」，失败方向是安全
//     的那一侧。）
//  2. 先解到目标目录里的一个暂存目录，再逐个搬进来。暂存目录由这一次请求独占，
//     归档里的符号链接够不到它外面；搬迁走 os.Root 的 rename，同样出不去。
//  3. 归档里的**符号链接与非普通文件一律不还原**（见 sanitizeExtracted）：链接可以指向归档外面，
//     留在用户的项目里既没有用处，又会被后面的复制 / 打包顺着读出去。
//  4. 所有写入都走 os.Root，不用拼绝对路径的 os.OpenFile。
//  5. 归档本身必须是 Root 里的**普通文件**（requireRegularFile）：交给外部命令的是
//     绝对路径（x.Abs），os.Root 在这一步帮不上忙，一个指向 Root 之外的符号链接就能
//     让解压把外部文件的内容读进项目里。
//  6. 解压结果里的**非普通文件一律删掉**（见 sanitizeExtracted）：tar 能造出 FIFO，
//     而 FIFO 在没人写的时候 open 会一直阻塞——留在项目里，之后每次读它 / 解压它都会
//     永久占住一个 goroutine 和一个子进程。
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
	UnzipBin = "/usr/bin/unzip"
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
	// Links 是归档里带、但被丢掉没还原的符号链接数（见 sanitizeExtracted）。
	Links int `json:"links"`
	// Specials 是归档里带、但被丢掉没还原的非普通文件数（FIFO、设备节点、socket…）。
	Specials int `json:"specials"`
}

// Extract 把 rel 指向的归档解压到它所在的目录。
//
// ctx 用来取消外部命令（客户端断开时不该留一个 tar 在后台跑）；超时由调用方给，
// 见 server 里的 extractTimeout。
func (x *Root) Extract(ctx context.Context, rel string) (ExtractResult, error) {
	var res ExtractResult

	rel, err := Rel(rel)
	if err != nil {
		return res, err
	}
	if rel == "." {
		return res, fmt.Errorf("%w：请选择一个压缩包", ErrInvalidPath)
	}
	// 归档本身必须是 Root 里的普通文件：外部命令吃的是绝对路径（x.Abs），沙箱帮不上忙。
	if err := x.requireRegularFile(rel); err != nil {
		return res, err
	}

	base := path.Base(rel)
	kind, ok := ArchiveKindOf(base)
	if !ok {
		return res, fmt.Errorf("%w：不支持的压缩包格式：%s", ErrInvalidPath, base)
	}
	res.Kind = string(kind)
	res.Dir = path.Dir(rel)

	staging := path.Join(res.Dir, fmt.Sprintf(".fpkstudio-extract-%d", time.Now().UnixNano()))
	// 走 x.Mkdir 而不是 x.r.Mkdir：飞牛卷上 mkdir(2) 的 mode 可能被忽略，这里要的
	// 0o700 只有事后 chmod 才落得下来（见 Root.Mkdir 的说明）。
	if err := x.Mkdir(staging, 0o700, false); err != nil {
		return res, fmt.Errorf("创建暂存目录失败: %w", err)
	}
	// 无论成败都清掉暂存目录：留下它只会让用户的目录里多一个看不懂的东西。
	defer func() { _ = x.r.RemoveAll(staging) }()

	switch {
	case kind.isTar():
		err = x.untar(ctx, x.Abs(rel), staging)
	case kind == KindZip:
		err = x.unzip(ctx, rel, staging)
	default:
		err = x.decompress(ctx, x.Abs(rel), base, staging, kind)
	}
	if err != nil {
		return res, err
	}

	// 链接与 FIFO / 设备节点都不还原、也不留：链接可以指向归档之外，FIFO 会让之后
	// 每一次读它都永久阻塞。数量报给界面，别让用户以为文件凭空少了。
	links, specials, err := x.sanitizeExtracted(staging)
	if err != nil {
		return res, err
	}
	res.Links = links
	res.Specials = specials

	if err := x.mergeInto(staging, res.Dir, &res); err != nil {
		return res, err
	}
	return res, nil
}

// sanitizeExtracted 删掉解压到 base 里的符号链接与非普通文件，返回两类删除的数量。
//
// 和 unpackFPK 里「不还原链接」是同一条策略，只是那边是手写 tar 解析时顺手跳过的。
// 这里必须显式做一次，因为 tar 和 unzip 默认都会把链接原样还原出来：
//
//	ln -s /etc/passwd leak
//
// 这样的链接留在用户的项目里，界面上点不进去（os.Root 拒绝穿出去），却会在打包 /
// 导出时被顺着读出去（CopyTree 打开的是链接指向的文件），内容就这么进了别人的 fpk。
//
// 非普通文件（FIFO / 设备节点 / socket）是另一码事，但危害更直接：FIFO 没有写者时
// open(2) 会一直阻塞，`tar -tf pipe` 与 `/api/fs/read` 都会**永久挂住**，一个请求占一个
// goroutine 加一个子进程，客户端断开也不结束。tar 造 FIFO 不需要 root，所以这不是
// 「反正也创建不了」的东西。删掉它、把数量报给界面，比留一个「点了就卡死」的条目清楚。
//
// 不往下递归：链接与非普通文件都不是目录（Lstat 说了算），Walk 不会跟着它们走。
func (x *Root) sanitizeExtracted(base string) (links, specials int, err error) {
	err = x.Walk(base, func(cur string, _ bool, _ string) error {
		fi, statErr := x.r.Lstat(cur)
		if statErr != nil {
			return nil
		}
		mode := fi.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			if rmErr := x.Remove(cur, false); rmErr != nil {
				return rmErr
			}
			links++
		case !mode.IsRegular() && !mode.IsDir():
			if rmErr := x.Remove(cur, false); rmErr != nil {
				return rmErr
			}
			specials++
		}
		return nil
	})
	return links, specials, err
}

// requireRegularFile 确认 rel 是 Root 里的普通文件。
//
// 用 Lstat：跟随链接就失去了意义——指向 Root 之外的链接正是要挡的那种。
func (x *Root) requireRegularFile(rel string) error {
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return err
	}
	// 三种都算「用户选错了目标」（400），不是服务端故障（500）。
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w：%s 是符号链接，不能解压（链接可能指向项目目录之外）", ErrInvalidPath, rel)
	}
	if fi.IsDir() {
		return fmt.Errorf("%w：%s 是目录，不能解压", ErrInvalidPath, rel)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w：%s 不是普通文件（FIFO / 设备节点不能解压）", ErrInvalidPath, rel)
	}
	return nil
}

// untar 先校验成员名，再用 tar 解到暂存目录（staging 是相对路径）。
//
// 不带 -z / -j / -J / --zstd：GNU tar 读归档时会按魔数认出压缩格式，一个命令覆盖
// 全部 tar 变体。
func (x *Root) untar(ctx context.Context, srcAbs, staging string) error {
	out, err := runTar(ctx, "-tf", srcAbs)
	if err != nil {
		return err
	}
	if err := checkMembers(out); err != nil {
		return err
	}

	if _, err := runTar(ctx, "-xf", srcAbs, "-C", x.Abs(staging)); err != nil {
		return err
	}
	return nil
}

func runTar(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, TarBin, "tar", args...)
}

// runUnzip 跑一次 unzip，返回 stdout（`-Z1` 的成员清单就在 stdout 上）。
func runUnzip(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, UnzipBin, "unzip", args...)
}

// runTool 执行一个解压工具，返回 stdout；失败时把输出的尾巴带进错误里。
//
// stdin 故意不接：这些工具在遇到「要不要覆盖」「密码是多少」时会等输入，而我们没有
// 终端可问——不接 stdin 时它们读到 EOF 会自己失败退出，好过把整个请求吊住。
func runTool(ctx context.Context, bin, name string, args ...string) ([]byte, error) {
	exe, err := binPath(bin)
	if err != nil {
		return nil, err
	}
	// CommandContext：请求被取消（客户端断开）或超时就把子进程杀掉，不留一个
	// 在后台继续跑的解压进程。
	cmd := exec.CommandContext(ctx, exe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// 有的工具把警告写在 stdout 上，两边都看一眼再取尾巴。
		detail := strings.TrimSpace(stderr.String() + "\n" + stdout.String())
		return nil, fmt.Errorf("%s %s 失败: %w: %s", name, strings.Join(args, " "), err, tailText(detail))
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
func (x *Root) decompress(ctx context.Context, srcAbs, base, staging string, kind ArchiveKind) error {
	tool, ok := singleTools[kind]
	if !ok {
		return fmt.Errorf("%w：不支持解压这种格式：%s", ErrInvalidPath, base)
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

	cmd := exec.CommandContext(ctx, bin, "-dc", srcAbs)
	cmd.Stdout = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("解压 %s 失败: %w: %s", base, err, tailText(stderr.String()))
	}
	return nil
}

// unzip 用系统的 unzip 把 zip 解到暂存目录。
//
// 走外部命令而不是标准库 archive/zip（按需求）：行为跟用户在终端里敲一遍一致，
// Info-ZIP 处理的那些边角（属性位、跨平台名字、加密包）不用我们再维护一份。
//
// 三件事必须做对：
//  1. 先 `unzip -Z1` 列出全部成员名逐个校验。**不能只看退出码**：实测 Info-ZIP 遇到
//     `../evil` / `/abs` 这类名字时会「警告 + 把名字截断成安全的样子」照写不误，
//     退出码只给 1，靠它挡不住（和 tar 那个坑一模一样）。
//  2. 必须先校验完再解压：一半解出来、一半被拒会留下一个说不清状态的目录。
//  3. `-o` 不能省。不给的话 unzip 遇到同名文件会停下来等 stdin 回话，而这里没有终端，
//     进程会一直挂着（暂存目录是这次请求独占的，覆盖没有风险）。加密包同理：没有
//     stdin，unzip 读不到密码就自己退出，不会把服务吊死。
//
// 符号链接与非普通文件由 Extract 在解压完之后统一丢掉（见 sanitizeExtracted）。
func (x *Root) unzip(ctx context.Context, rel, staging string) error {
	src := x.Abs(rel)

	out, err := runUnzip(ctx, "-Z1", src)
	if err != nil {
		// 列不出成员，多半是「不是 zip」或者「空包」——unzip 的原话在错误尾巴里，
		// 这里补一句人话，别让用户只看到 `unzip -Z1 … 失败`。
		return fmt.Errorf("读不出 zip 的成员清单（可能不是有效的 zip，或者包是空的）: %w", err)
	}
	if err := checkMembers(out); err != nil {
		return err
	}

	if _, err := runUnzip(ctx, "-qq", "-o", "-d", x.Abs(staging), src); err != nil {
		return err
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
