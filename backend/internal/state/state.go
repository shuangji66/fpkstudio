// Package state 持久化「跟着安装走」的少量设置。
//
// 和 config 的分工：config 解析环境变量（部署形态，进程启动时定下），state 保存
// 运行时能被用户改掉的那几项（当前是项目根目录与 fpk 输出目录），落在数据目录
// 下的 state.json。文件必须落在数据目录里：换部署目录时设置跟着走，删数据目录
// 就等于恢复默认。
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Dirs 是两个可配置目录。空字符串表示「没设置过」，用回环境变量给的默认值。
type Dirs struct {
	// ProjectRoot 是项目根目录（项目列表与文件树的沙箱根）。
	ProjectRoot string `json:"projectRoot"`
	// OutputDir 是打包产物落地目录。
	OutputDir string `json:"outputDir"`
}

// Empty 判断两个目录都没设置。
func (d Dirs) Empty() bool { return d.ProjectRoot == "" && d.OutputDir == "" }

// Store 读写 state.json。零值不可用，用 Open 构造。
type Store struct {
	path string

	mu  sync.Mutex
	cur Dirs
}

// Open 读取状态文件。文件不存在算「空配置」，不算错误；文件内容坏掉时报错，
// 但调用方通常只记一条日志、按默认目录继续跑（不能因为一个坏 JSON 起不来）。
func Open(path string) (*Store, error) {
	s := Empty(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	var d Dirs
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	s.cur = d.clean()
	return s, nil
}

// Empty 返回一个不含任何设置的 Store（读坏了的时候用它继续跑）。
func Empty(path string) *Store { return &Store{path: path} }

// Path 返回状态文件路径。
func (s *Store) Path() string { return s.path }

// Dirs 返回当前保存的目录设置。
func (s *Store) Dirs() Dirs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

// Save 写入目录设置。
//
// 先写同目录下的临时文件再 rename：这个文件是「启动时读一次」的，写到一半被
// 打断会留下一个坏 JSON，下次启动就得靠人去删。rename 在同一个目录内是原子的。
func (s *Store) Save(d Dirs) error {
	d = d.clean()

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	// 飞牛卷上 open(O_CREAT, mode) 的 mode 会被忽略，新建文件可能落成 0000，
	// 这里显式补一次权限（见 fsx 里的同一处说明）。
	if err := os.Chmod(tmp, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	s.cur = d
	return nil
}

// clean 统一路径写法：去掉首尾空白，其余交给调用方（projects 会再校验一遍）。
func (d Dirs) clean() Dirs {
	d.ProjectRoot = strings.TrimSpace(d.ProjectRoot)
	d.OutputDir = strings.TrimSpace(d.OutputDir)
	return d
}
