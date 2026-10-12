// Package config 负责 fpkstudio 的全部运行时配置。
//
// 设计原则：所有可变路径都由环境变量统一提供，程序自身不假设任何部署形态。
// 数据目录（projects / logs / run / tmp / state.json）与打包输出目录分开配置，
// 日志、PID、socket、临时文件全部落在数据目录下。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fpkstudio/internal/trimapi"
)

// 环境变量名。所有变量都以 FPKSTUDIO_ 为前缀。
const (
	EnvDataDir     = "FPKSTUDIO_DATA_DIR"
	EnvProjectsDir = "FPKSTUDIO_PROJECTS_DIR"
	EnvOutputDir   = "FPKSTUDIO_OUTPUT_DIR"
	EnvSocket      = "FPKSTUDIO_SOCKET"
	EnvBaseURL     = "FPKSTUDIO_BASE_URL"
	EnvFnpack      = "FPKSTUDIO_FNPACK"
	EnvLogLevel    = "FPKSTUDIO_LOG_LEVEL"
	EnvRingSize    = "FPKSTUDIO_LOG_RING"
	EnvTrimSocket  = "FPKSTUDIO_TRIM_SOCKET"
	EnvTrimApp     = "FPKSTUDIO_TRIM_APP"
)

// unix socket 的 sun_path 上限（Linux）。留一点余量给结尾的 \0。
const maxSocketPath = 100

// Config 是解析完成的运行时配置。
type Config struct {
	Version string

	// DataDir 是数据根目录，下面挂 ProjectsDir/LogDir/RunDir/TmpDir。
	DataDir string
	// OutputDir 是打包产物目录，允许在 DataDir 之外（例如挂到共享目录）。
	OutputDir string

	// ProjectsDir 是项目根目录，允许在 DataDir 之外（例如指向用户自己的存储空间）。
	ProjectsDir string
	LogDir      string
	RunDir      string
	TmpDir      string

	SocketPath string
	PIDFile    string
	StateFile  string

	// BaseURL 形如 "/" 或 "/app/fpkstudio"，永远不会以 "/" 结尾（根除外）。
	BaseURL string

	// FnpackBin 是 fnpack 可执行文件的路径或名字。
	FnpackBin string
	LogLevel  string
	LogRing   int

	// OutputEnvIgnored 为真表示 FPKSTUDIO_OUTPUT_DIR 被忽略了（它和项目目录指到
	// 同一个路径），OutputDir 已经回退到数据目录下的默认值。界面据此标注。
	OutputEnvIgnored bool

	// TrimSocket 是飞牛开放 API 的 unix socket（默认由系统提供）。
	TrimSocket string
	// TrimApp 是本应用在飞牛系统里的应用名，开放 API 请求体里的 appName。
	TrimApp string
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// NormalizeBaseURL 把任意用户输入规整成 "/" 或 "/a/b" 的形式。
func NormalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "//", "/")
	if s == "" || s == "/" {
		return "/"
	}
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "/"
	}
	return s
}

// Load 读取环境变量、创建目录、写出 PID 文件之外的一切准备工作。
func Load(version string) (*Config, error) {
	dataDir, err := filepath.Abs(env(EnvDataDir, defaultDataDir()))
	if err != nil {
		return nil, fmt.Errorf("解析数据目录失败: %w", err)
	}

	c := &Config{
		Version:   version,
		DataDir:   filepath.Clean(dataDir),
		OutputDir: cleanAbs(env(EnvOutputDir, filepath.Join(dataDir, "output"))),
		BaseURL:   NormalizeBaseURL(env(EnvBaseURL, "/")),
		FnpackBin: env(EnvFnpack, "fnpack"),
		LogLevel:  strings.ToLower(env(EnvLogLevel, "info")),
		LogRing:   atoiDefault(env(EnvRingSize, ""), 5000),
		// 默认值来自飞牛开发文档：socket 由系统提供，appName 就是 fpk 的
		// manifest.appname。两者都允许覆盖，方便在没有 fnOS 的环境里调链路。
		TrimSocket: env(EnvTrimSocket, trimapi.DefaultSocket),
		TrimApp:    env(EnvTrimApp, "fpkstudio"),
	}

	c.ProjectsDir = cleanAbs(env(EnvProjectsDir, filepath.Join(c.DataDir, "projects")))
	// 项目目录与输出目录指到同一个路径时保项目目录，输出目录按「没设置」处理、
	// 退回默认值：两个目录混在一起会让产物既算项目产物又算输出产物，列两遍、复制
	// 到自己身上，谁都说不清它到底在哪。
	if samePath(c.ProjectsDir, c.OutputDir) {
		c.OutputEnvIgnored = true
		c.OutputDir = filepath.Join(c.DataDir, "output")
	}

	c.LogDir = filepath.Join(c.DataDir, "logs")
	c.RunDir = filepath.Join(c.DataDir, "run")
	c.TmpDir = filepath.Join(c.DataDir, "tmp")
	c.SocketPath = cleanAbs(env(EnvSocket, filepath.Join(c.RunDir, "fpkstudio.sock")))
	c.PIDFile = filepath.Join(c.RunDir, "fpkstudio.pid")
	c.StateFile = filepath.Join(c.DataDir, "state.json")

	for _, d := range []string{c.DataDir, c.OutputDir, c.ProjectsDir, c.LogDir, c.RunDir, c.TmpDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("创建目录 %s 失败: %w", d, err)
		}
		// 飞牛卷上 O_CREAT / mkdir 的 mode 会被忽略（新建项继承父目录模式，访问权由
		// ACL 给），所以显式补一次——和 fsx 里的同一条约定。
		if err := os.Chmod(d, 0o755); err != nil {
			return nil, fmt.Errorf("设置目录 %s 的权限失败: %w", d, err)
		}
	}

	if len(c.SocketPath) > maxSocketPath {
		return nil, fmt.Errorf(
			"unix socket 路径过长（%d 字节，上限 %d）：%s\n请用 %s 指定一个更短的路径，例如 /tmp/fpkstudio.sock",
			len(c.SocketPath), maxSocketPath, c.SocketPath, EnvSocket)
	}
	return c, nil
}

// LogFile 返回主日志文件路径。
func (c *Config) LogFile() string { return filepath.Join(c.LogDir, "fpkstudio.log") }

// Env 返回用于注入子进程的环境变量，让 fnpack 把临时文件写在数据目录里。
//
// 这不是洁癖：fnpack 会在 TMPDIR 里复制整个项目再打包，如果 TMPDIR 跨挂载点
// 或者落在受限目录（某些 NAS 系统上 /tmp 有 ACL 限制），fnpack 会在
// "Copy pack ... to tmp dir" 这一步直接失败。
func (c *Config) Env() []string {
	env := os.Environ()
	env = append(env,
		"TMPDIR="+c.TmpDir,
		"TMP="+c.TmpDir,
		"TEMP="+c.TmpDir,
	)
	return env
}

func defaultDataDir() string {
	if x := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); x != "" {
		return filepath.Join(x, "fpkstudio")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".local", "share", "fpkstudio")
	}
	return "fpkstudio-data"
}

// samePath 判断两个路径是不是同一个位置（只做清洁后的字面比较，不去解析符号
// 链接：这里判的是「部署时填重了」，不是「文件系统上是不是同一个 inode」）。
func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func cleanAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(a)
}

// atoiDefault 解析一个正整数配置，非法或超范围时回落到默认值。
//
// 以前是手写累加：`FPKSTUDIO_LOG_RING=99999999999999999999` 会溢出成一个巨大的正数，
// 接着 logging.New 的 `make([]Entry, 0, n)` 直接把进程打死（配置写错不该让服务起不来，
// 更不该以 panic 的形式）。
func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > maxConfigInt {
		return def
	}
	return n
}

// maxConfigInt 是数字型配置项的上限（防手滑写一个天文数字把内存吃光）。
const maxConfigInt = 1 << 20
