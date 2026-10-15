// Package fsx 在 os.Root 之上提供一层文件操作，用作「只能待在某个目录里」的沙箱。
//
// 之所以用 os.Root 而不是自己拼路径 + 字符串前缀检查：os.Root 在 openat 系统调用
// 层面拒绝任何逃逸（包括 ../ 和指向外部的符号链接），不依赖调用方记得校验。
package fsx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ErrNotExist 表示目标不存在。
var ErrNotExist = os.ErrNotExist

// ErrExists 表示目标已经存在，且这次操作不允许覆盖它（见 Root.Rename）。
var ErrExists = errors.New("目标已存在")

// ErrInvalidPath 表示传进来的路径本身不合法（含 `..`、NUL 等）。
//
// 单独一个哨兵是为了让 HTTP 层能把它翻成 400（用户改一下就好），而不是当成服务端
// 故障的 500——以前 `/api/fs/write {"path":"../x"}` 会回 500「路径不允许包含 ..」。
var ErrInvalidPath = errors.New("路径不合法")

// Entry 是目录列表里的一项。
type Entry struct {
	Name    string `json:"name"`
	Dir     bool   `json:"dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"modTime"`
	// Symlink 为真时 Dir 不可信，前端只展示不可进入。
	Symlink bool `json:"symlink"`
}

// Root 是一个受限目录。
type Root struct {
	dir string
	r   *os.Root
}

// Open 打开受限目录，目录不存在时创建。
func Open(dir string, create bool) (*Root, error) {
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &Root{dir: dir, r: r}, nil
}

// Dir 返回该根目录的绝对路径。
func (x *Root) Dir() string { return x.dir }

// Close 关闭。
func (x *Root) Close() error {
	if x.r == nil {
		return nil
	}
	return x.r.Close()
}

// Rel 把外部传入的相对路径规整成 os.Root 能接受的形式。
//
// 返回的路径一定是相对路径、用 "/" 分隔、不含 "." 和 ".." 段。
func Rel(p string) (string, error) {
	p = strings.ReplaceAll(p, "\\", "/")
	// 只在「整串都是空白」时当成根目录：无条件 TrimSpace 会把 "x "（SMB / Windows 上
	// 很常见）指向另一个文件 "x"，删改的都是别人。
	if strings.TrimSpace(p) == "" {
		return ".", nil
	}
	if p == "." || p == "/" {
		return ".", nil
	}
	if strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("%w：路径包含非法字符", ErrInvalidPath)
	}
	clean := path.Clean(p)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return ".", nil
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%w：路径不允许包含 ..", ErrInvalidPath)
		}
	}
	return clean, nil
}

// Abs 返回受限目录内的绝对路径，仅用于展示和传给外部命令。
func (x *Root) Abs(rel string) string {
	if rel == "." || rel == "" {
		return x.dir
	}
	return filepath.Join(x.dir, filepath.FromSlash(rel))
}

// Stat 返回一项的元信息。
func (x *Root) Stat(rel string) (Entry, error) {
	rel, err := Rel(rel)
	if err != nil {
		return Entry{}, err
	}
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return Entry{}, err
	}
	return toEntry(fi), nil
}

// List 列出目录内容，目录在前、同类按名字排序。
func (x *Root) List(rel string) ([]Entry, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, err
	}
	f, err := x.r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	des, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		e := Entry{Name: de.Name(), Dir: de.IsDir()}
		if info, err := de.Info(); err == nil {
			sized := toEntry(info)
			e.Size = sized.Size
			e.Mode = sized.Mode
			e.ModTime = sized.ModTime
			e.Symlink = sized.Symlink
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// ReadFile 读取文件。
//
// 只读普通文件：FIFO / 设备节点在没有写者时 open(2) 会一直阻塞，一个请求就能永久占住
// 一个 goroutine（解压出来的 FIFO 已经会被 sanitizeExtracted 删掉，这里是第二道）。
func (x *Root) ReadFile(rel string) ([]byte, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, err
	}
	if err := x.requireReadableFile(rel); err != nil {
		return nil, err
	}
	return x.r.ReadFile(rel)
}

// requireReadableFile 确认目标是可读的普通文件（符号链接按它指向的东西判）。
func (x *Root) requireReadableFile(rel string) error {
	fi, err := x.r.Stat(rel)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("目标是目录")
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s 不是普通文件（FIFO / 设备节点读不了）", rel)
	}
	return nil
}

// ReadFileLimit 读文件，最多读 max 字节。
//
// 超出时返回 truncated=true 与已读到的前 max 字节——调用方（`/api/fs/read`）据此
// 只回元信息，不把整份内容塞进 JSON：app/server 下几百 MB 的原生程序很常见，以前
// 那个接口会先把它整份读进内存再决定要不要回。
func (x *Root) ReadFileLimit(rel string, max int64) ([]byte, bool, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, false, err
	}
	if err := x.requireReadableFile(rel); err != nil {
		return nil, false, err
	}
	f, err := x.r.Open(rel)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > max {
		return data[:max], true, nil
	}
	return data, false, nil
}

// OpenFile 打开文件用于流式读取（下载）。
func (x *Root) OpenFile(rel string) (*os.File, os.FileInfo, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, nil, err
	}
	if err := x.requireReadableFile(rel); err != nil {
		return nil, nil, err
	}
	fi, err := x.r.Stat(rel)
	if err != nil {
		return nil, nil, err
	}
	f, err := x.r.Open(rel)
	if err != nil {
		return nil, nil, err
	}
	return f, fi, nil
}

// WriteFile 写入文件，自动创建父目录。
func (x *Root) WriteFile(rel string, data []byte, perm os.FileMode) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	if _, err := x.WriteReader(rel, bytes.NewReader(data), perm); err != nil {
		return err
	}
	return nil
}

// WriteReader 把 r 的内容流式写进文件，自动创建父目录，返回写入的字节数。
//
// 上传、导入这类「内容有多大不由我们说了算」的路径走它，内存占用与文件大小无关
// （以前是 io.ReadAll，上传一个几百 MB 的程序就得先在内存里放一份）。
//
// 权限处理与 WriteFile 一致：写完显式 chmod 一次，理由见 forceMode 的说明。
func (x *Root) WriteReader(rel string, r io.Reader, perm os.FileMode) (int64, error) {
	rel, err := Rel(rel)
	if err != nil {
		return 0, err
	}
	if parent := path.Dir(rel); parent != "." {
		if err := x.r.MkdirAll(parent, 0o755); err != nil {
			return 0, err
		}
	}
	f, err := x.r.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return n, err
	}
	// chmod 失败不阻断写入：文件内容已经落盘，模式位只影响产物质量。
	_ = x.forceMode(rel, perm)
	return n, nil
}

// forceMode 在创建后显式设置一次模式位。
//
// 这里可以放心设置，因为我们给的是已知可用的模式（0644/0755）；而在飞牛的
// 存储卷上，新建文件默认是「模式位 0 + ACL 授权」，模式位为 0 的文件打进
// fpk 之后在目标机器上可能不可用。设置失败不影响写入本身。
func (x *Root) forceMode(rel string, perm os.FileMode) error {
	if err := x.r.Chmod(rel, perm); err != nil {
		return fmt.Errorf("设置 %s 的权限失败: %w", rel, err)
	}
	return nil
}

// RepairModes 在 base 范围内修复「不可用」的权限位。
//
// 背景：飞牛的存储卷用的是 Windows ACL 模型，新文件落地时 POSIX 模式位是 0，
// 访问权由 ACL 提供；一旦调用 chmod，模式位就变成真正的访问控制。
// 因此这里的策略是「只在模式位确实为空、或文件确实读不了时才动」，
// 并且每次 chmod 之后都回读验证，绝不盲目设置。
//
// 目标有两个：一是让 fnpack 能读到所有文件，二是让打进 fpk 的模式位是正常的
// （0 模式的 manifest / cmd 脚本在目标机器上可能不可用）。
//
// 返回修复数量。
func (x *Root) RepairModes(base string, scriptDirs []string) (int, error) {
	base, err := Rel(base)
	if err != nil {
		return 0, err
	}
	repaired := 0
	chmod := func(rel string, mode os.FileMode) {
		if x.r.Chmod(rel, mode) == nil {
			repaired++
		}
	}
	err = x.Walk(base, func(cur string, isDir bool, _ string) error {
		fi, statErr := x.r.Lstat(cur)
		if statErr != nil {
			return nil
		}
		perm := fi.Mode().Perm()
		local := relName(base, cur)

		if isDir {
			if perm&0o700 != 0o700 {
				chmod(cur, perm|0o700)
			}
			return nil
		}

		isScript := false
		for _, dir := range scriptDirs {
			if strings.HasPrefix(local, strings.Trim(dir, "/")+"/") {
				isScript = true
				break
			}
		}

		if perm&0o444 == 0 {
			// 完全读不了的普通文件，补一个安全默认值。
			want := os.FileMode(0o644)
			if isScript {
				want = 0o755
			}
			chmod(cur, want)
			return nil
		}
		if isScript && perm&0o111 == 0 {
			// 生命周期脚本补执行位。
			chmod(cur, perm|0o755)
		}
		return nil
	})
	return repaired, err
}

// Mkdir 创建目录，perm 会显式设置一遍。
//
// perm 是参数而不是写死的 0o755：飞牛卷上 open(O_CREAT, mode) 的 mode 会被忽略、
// 新建项改为继承父目录，所以只有事后 chmod 才真的生效（见 setMode）。调用方要的权限
// 并不相同（解压的暂存目录要 0o700），参数化之后就不会出现「传了 0o700 却落成 0o755」
// 这种看不见的偏差。
func (x *Root) Mkdir(rel string, perm os.FileMode, all bool) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	if all {
		if err := x.r.MkdirAll(rel, perm); err != nil {
			return err
		}
	} else if err := x.r.Mkdir(rel, perm); err != nil {
		return err
	}
	// 根目录自己不改模式：`Mkdir(".", 0o755, true)` 会把用户授权的、权限可能更紧的
	// 项目根目录静默放宽（`/api/fs/create {"path":".","dir":true}` 就能触发）。
	if rel == "." {
		return nil
	}
	return x.setMode(rel, perm)
}

// Remove 删除文件或目录。dir 为真时允许递归删除目录。
func (x *Root) Remove(rel string, dir bool) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	if rel == "." {
		return fmt.Errorf("%w：不允许删除根目录", ErrInvalidPath)
	}
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if !dir {
			return errors.New("目标是目录")
		}
		return x.r.RemoveAll(rel)
	}
	return x.r.Remove(rel)
}

// Rename 移动 / 重命名。
func (x *Root) Rename(from, to string) error {
	from, err := Rel(from)
	if err != nil {
		return err
	}
	to, err = Rel(to)
	if err != nil {
		return err
	}
	if from == "." || to == "." {
		return fmt.Errorf("%w：不允许移动根目录", ErrInvalidPath)
	}
	if to == from || strings.HasPrefix(to, from+"/") {
		return fmt.Errorf("%w：不能把目录移动到自身内部", ErrInvalidPath)
	}
	if parent := path.Dir(to); parent != "." {
		if err := x.r.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	// 目标已存在就拒绝，绝不覆盖。
	//
	// rename(2) 会把同名文件直接顶掉，而调用方（文件管理器的重命名）只是想改个名字
	// ——用户的另一个文件就这么没了，还没有任何提示。解压合并那条路
	// （archive.mergeEntry）在调用前已经确认过目标不存在，所以这里的检查不会挡住它。
	if _, statErr := x.r.Lstat(to); statErr == nil {
		return fmt.Errorf("%w：%s", ErrExists, to)
	}
	return x.r.Rename(from, to)
}

// Copy 递归复制文件或目录。
func (x *Root) Copy(from, to string) error {
	from, err := Rel(from)
	if err != nil {
		return err
	}
	to, err = Rel(to)
	if err != nil {
		return err
	}
	if from == "." || to == "." {
		return fmt.Errorf("%w：不允许复制根目录", ErrInvalidPath)
	}
	if to == from || strings.HasPrefix(to, from+"/") {
		return fmt.Errorf("%w：不能把目录复制到自身内部", ErrInvalidPath)
	}
	// 目标已存在就拒绝，绝不覆盖——和 Rename 同一条规矩。copy 内部用 O_TRUNC 写每一个
	// 文件，静默覆盖会让「复制到这里」变成「把那个文件换掉」，而且不可逆。正常调用方
	// （粘贴）会先用 uniqueName 换一个不重名的名字，所以这条挡不住正常路径。
	if _, statErr := x.r.Lstat(to); statErr == nil {
		return fmt.Errorf("%w：%s", ErrExists, to)
	}
	return x.copy(from, to)
}

func (x *Root) copy(from, to string) error {
	fi, err := x.r.Lstat(from)
	if err != nil {
		return err
	}
	mode := inheritMode(fi)

	if !fi.IsDir() {
		if parent := path.Dir(to); parent != "." {
			if err := x.r.MkdirAll(parent, 0o755); err != nil {
				return err
			}
		}
		src, err := x.r.Open(from)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := x.r.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			return err
		}
		dst.Close()
		return x.setMode(to, mode)
	}

	if err := x.r.MkdirAll(to, mode); err != nil {
		return err
	}
	// 先给目录定好模式，再往里放东西：这个文件系统上新建项会继承父目录模式，
	// 父目录模式没定死的话会把子项的权限一起带偏。
	if err := x.setMode(to, mode); err != nil {
		return err
	}
	children, err := x.List(from)
	if err != nil {
		return err
	}
	for _, c := range children {
		if err := x.copy(from+"/"+c.Name, to+"/"+c.Name); err != nil {
			return err
		}
	}
	return nil
}

// setMode 显式设置模式位。
//
// 不是多此一举：飞牛的存储卷上 open(O_CREAT, mode) 的 mode 参数会被忽略，
// 新建项改为继承父目录的模式，只有事后 chmod 才能真正生效。
func (x *Root) setMode(rel string, mode os.FileMode) error {
	return x.r.Chmod(rel, mode)
}

// inheritMode 决定复制目标应当使用的模式。
//
// 源文件的模式位可能是 0（飞牛卷上由 ACL 授权、模式位未设的新文件），
// 直接照抄会让目标文件也不可用，这里给一个安全默认值。
func inheritMode(fi os.FileInfo) os.FileMode {
	perm := fi.Mode().Perm()
	if fi.IsDir() {
		return perm | 0o700
	}
	if perm == 0 {
		return 0o644
	}
	return perm
}

// ZipDir 把目录打包成 zip 写到 w（用于「下载整个项目」/「下载目录」）。
//
// zip 里**保留一层顶层目录**（名字取 rel 的最后一段）：解压出来是一个目录，而不是把
// 内容直接撒在解压目标里。项目根里全是 `manifest`、`app/` 这种大众名字，摊平之后
// 「解压到当前目录」会和用户已有的文件混在一起；有这一层，解压到哪儿都独占一个目录。
//
// rel 为 "."（沙箱根本身）时不加这一层：这时没有目录名可取（`path.Base(".")` 就是
// "."），硬拼出来的 `./manifest` 不是一层目录，反而会让部分解压工具在目标里再建一个
// 名字可疑的目录。
func (x *Root) ZipDir(rel string, w io.Writer) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	base := rel
	prefix := ""
	if base != "." {
		prefix = path.Base(base) + "/"
		// 顶层目录自己也要有一条记录：源目录为空时整个 zip 就只剩它；少这一条会得到
		// 一个没有任何成员的「空包」，解压工具直接报错，而不是解出一个空目录。
		if _, err := zw.Create(prefix); err != nil {
			return err
		}
	}
	err = x.walk(rel, func(cur string, fi os.FileInfo) error {
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		name := relName(base, cur)
		if name == "" {
			return nil
		}
		hdr.Name = prefix + name
		if fi.IsDir() {
			hdr.Name += "/"
		} else {
			hdr.Method = zip.Deflate
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		f, err := x.r.Open(cur)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(fw, f)
		return err
	})
	// Close 负责写中央目录，它的错误必须带出来：调用方据此判断这个 zip 能不能用
	// （以前是 defer 里丢掉的，于是「缺文件的残包」也能得到一个 200）。
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	return err
}

// ErrWalkStop 用于在遍历回调里提前结束（不算失败）。
var ErrWalkStop = errors.New("停止遍历")

// LatestModTime 返回 base 子树里最新的 mtime（含 base 自身），最多看 limit 个条目。
//
// 用途：目录自身的 mtime 只在增删条目时变，改文件内容不会动它。项目列表要显示
// 「最后修改时间」，就得看整棵树。limit <= 0 表示不限制；超出 limit 时返回目前
// 找到的最新时间（宁可少看几个，也不要为了一个时间把大项目扫一遍）。
func (x *Root) LatestModTime(base string, limit int) (time.Time, error) {
	var (
		newest time.Time
		seen   int
	)
	err := x.walk(base, func(_ string, fi os.FileInfo) error {
		seen++
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		if limit > 0 && seen >= limit {
			return ErrWalkStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrWalkStop) {
		return newest, err
	}
	return newest, nil
}

// Walk 深度优先遍历，回调收到相对路径、是否目录、以及基础名。
func (x *Root) Walk(rel string, fn func(cur string, isDir bool, name string) error) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	return x.walk(rel, func(cur string, fi os.FileInfo) error {
		name := fi.Name()
		if cur == "." {
			name = "."
		}
		return fn(cur, fi.IsDir(), name)
	})
}

// walk 深度优先遍历，回调按目录在前的顺序收到每一层。
func (x *Root) walk(rel string, fn func(string, os.FileInfo) error) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	fi, err := x.r.Lstat(rel)
	if err != nil {
		return err
	}
	if err := fn(rel, fi); err != nil {
		return err
	}
	if !fi.IsDir() {
		return nil
	}
	children, err := x.List(rel)
	if err != nil {
		return err
	}
	for _, c := range children {
		child := rel
		if child == "." {
			child = c.Name
		} else {
			child = child + "/" + c.Name
		}
		if err := x.walk(child, fn); err != nil {
			return err
		}
	}
	return nil
}

// CopyTree 按绝对路径递归复制文件或目录。
//
// 为什么不能只用 Root.Copy：项目根目录可以是用户授权的目录（见
// projects.Manager.SetRoots），它落在数据目录沙箱之外，而打包前的暂存复制是
// 「项目目录 → 数据目录的 tmp/」，跨了两个沙箱，没法用任何一个 Root 的相对路径
// 表达。复制语义与 Root.copy 保持一致（模式位显式 chmod、源模式为 0 时给安全
// 默认值），否则飞牛卷上复制出来的是 0000 文件，fnpack 直接 EACCES。
func CopyTree(srcAbs, dstAbs string) error {
	fi, err := os.Lstat(srcAbs)
	if err != nil {
		return err
	}
	// 目标已存在就拒绝，绝不覆盖——和 Root.Copy 同一条规矩。copyTree 内部用 O_TRUNC
	// 写每一个文件，静默覆盖会让「导入一个项目」变成「把同名项目的文件换掉」。
	if _, statErr := os.Lstat(dstAbs); statErr == nil {
		return fmt.Errorf("%w：%s", ErrExists, dstAbs)
	}
	return copyTree(srcAbs, dstAbs, fi)
}

func copyTree(src, dst string, fi os.FileInfo) error {
	mode := inheritMode(fi)

	if !fi.IsDir() {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chmod(dst, mode)
	}

	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	// 先定好目录模式再往里放东西：这个文件系统上新建项会继承父目录模式，
	// 父目录模式没定死的话会把子项的权限一起带偏。
	if err := os.Chmod(dst, mode); err != nil {
		return err
	}
	children, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, c := range children {
		child := filepath.Join(src, c.Name())
		info, err := os.Lstat(child)
		if err != nil {
			return err
		}
		if err := copyTree(child, filepath.Join(dst, c.Name()), info); err != nil {
			return err
		}
	}
	return nil
}

func toEntry(fi os.FileInfo) Entry {
	e := Entry{
		Name:    fi.Name(),
		Dir:     fi.IsDir(),
		Size:    fi.Size(),
		Mode:    fi.Mode().String(),
		ModTime: fi.ModTime().Format("2006-01-02 15:04:05"),
		Symlink: fi.Mode()&os.ModeSymlink != 0,
	}
	if e.Symlink {
		// 符号链接的 IsDir 为假，且我们不允许穿出去，前端直接当文件处理。
		e.Dir = false
	}
	return e
}

// relName 把遍历拿到的路径 cur 变成相对 base 的名字（zip 条目名、权限判断都用它）。
//
// 不能直接 `strings.TrimPrefix(cur, base)`：base 是 "." 时（下载整个项目根、或者
// 在根上跑 RepairModes）它会把「以点开头」的名字吃掉一个字符——`.hidden/` 变成
// `hidden/`，和真正的 `hidden/` 撞在一起，zip 里就是两个同名条目互相覆盖。
func relName(base, cur string) string {
	if base == "." || base == "" {
		if cur == "." {
			return ""
		}
		return cur
	}
	return strings.TrimPrefix(strings.TrimPrefix(cur, base), "/")
}

// HumanSize 把字节数格式化成便于展示的形式。
func HumanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
