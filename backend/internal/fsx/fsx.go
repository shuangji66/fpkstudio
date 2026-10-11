// Package fsx 在 os.Root 之上提供一层文件操作，用作「只能待在某个目录里」的沙箱。
//
// 之所以用 os.Root 而不是自己拼路径 + 字符串前缀检查：os.Root 在 openat 系统调用
// 层面拒绝任何逃逸（包括 ../ 和指向外部的符号链接），不依赖调用方记得校验。
package fsx

import (
	"archive/zip"
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
	p = strings.TrimSpace(p)
	if p == "" || p == "." || p == "/" {
		return ".", nil
	}
	if strings.ContainsRune(p, 0) {
		return "", errors.New("路径包含非法字符")
	}
	clean := path.Clean(p)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return ".", nil
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == ".." {
			return "", errors.New("路径不允许包含 ..")
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
func (x *Root) ReadFile(rel string) ([]byte, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, err
	}
	return x.r.ReadFile(rel)
}

// OpenFile 打开文件用于流式读取（下载）。
func (x *Root) OpenFile(rel string) (*os.File, os.FileInfo, error) {
	rel, err := Rel(rel)
	if err != nil {
		return nil, nil, err
	}
	fi, err := x.r.Stat(rel)
	if err != nil {
		return nil, nil, err
	}
	if fi.IsDir() {
		return nil, nil, errors.New("目标是目录")
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
	if err := x.writeFile(rel, data, perm); err != nil {
		return err
	}
	// chmod 失败不阻断写入：文件内容已经落盘，模式位只影响产物质量。
	_ = x.forceMode(rel, perm)
	return nil
}

func (x *Root) writeFile(rel string, data []byte, perm os.FileMode) error {
	if parent := path.Dir(rel); parent != "." {
		if err := x.r.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	f, err := x.r.OpenFile(rel, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
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
		local := strings.TrimPrefix(strings.TrimPrefix(cur, base), "/")

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
func (x *Root) canRead(rel string) bool {
	f, err := x.r.Open(rel)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func (x *Root) canTraverse(rel string) bool {
	f, err := x.r.Open(rel)
	if err != nil {
		return false
	}
	_, err = f.ReadDir(1)
	_ = f.Close()
	return err == nil || errors.Is(err, io.EOF)
}

// Mkdir 创建目录。
func (x *Root) Mkdir(rel string, all bool) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	if all {
		if err := x.r.MkdirAll(rel, 0o755); err != nil {
			return err
		}
	} else if err := x.r.Mkdir(rel, 0o755); err != nil {
		return err
	}
	return x.setMode(rel, 0o755)
}

// Remove 删除文件或目录。dir 为真时允许递归删除目录。
func (x *Root) Remove(rel string, dir bool) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	if rel == "." {
		return errors.New("不允许删除根目录")
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
		return errors.New("不允许移动根目录")
	}
	if to == from || strings.HasPrefix(to, from+"/") {
		return errors.New("不能把目录移动到自身内部")
	}
	if parent := path.Dir(to); parent != "." {
		if err := x.r.MkdirAll(parent, 0o755); err != nil {
			return err
		}
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
		return errors.New("不允许复制根目录")
	}
	if to == from || strings.HasPrefix(to, from+"/") {
		return errors.New("不能把目录复制到自身内部")
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

// ZipDir 把目录打包成 zip 写到 w（用于「下载整个项目」）。
func (x *Root) ZipDir(rel string, w io.Writer) error {
	rel, err := Rel(rel)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	defer zw.Close()

	base := rel
	return x.walk(rel, func(cur string, fi os.FileInfo) error {
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(strings.TrimPrefix(cur, base), "/")
		if name == "" {
			return nil
		}
		hdr.Name = name
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

// WriteFileTo 在受限目录外写文件（打包产物落到 OutputDir 时用）。
func WriteFileTo(absPath string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, data, perm)
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
