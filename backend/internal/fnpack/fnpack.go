// Package fnpack 包装飞牛官方的 fnpack 命令行工具。
//
// 只做三件事：定位、执行、把输出逐行转给日志器。
//
// 成败判断以退出码为准，`build` 是这样的。`create` 不是：项目名非法时（太短、
// 含非法字符）它照样以退出码 0 结束，只把错误打在 stdout 上。所以调用 create
// 的一方除了退出码，还得回读输出——见 FailureReason。
package fnpack

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"fpkstudio/internal/config"
	"fpkstudio/internal/logging"
)

var versionRe = regexp.MustCompile(`(?i)version\s+v?([0-9][0-9A-Za-z.\-]*)`)

// create 失败时的输出形如：
//
//	Create application name ab error : appname ilegal,app name (ab) length must be between 3 and 32
var failureRe = regexp.MustCompile(`(?i)\berror\b\s*:`)

// FailureReason 从命令输出里取出 fnpack 自己给出的失败原因，没有则返回空串。
//
// 存在的原因只有一个：fnpack create 失败时退出码还是 0，判断成败只能看输出。
func FailureReason(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" && failureRe.MatchString(line) {
			return line
		}
	}
	return ""
}

// Client 是 fnpack 的调用入口。
type Client struct {
	cfg *config.Config
	log *logging.Logger

	mu          sync.Mutex
	binPath     string
	version     string
	probeResult string
}

// New 创建客户端。
func New(cfg *config.Config, log *logging.Logger) *Client {
	return &Client{cfg: cfg, log: log}
}

// Bin 返回实际可执行文件路径（探测失败时返回原始配置值）。
func (c *Client) Bin() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.binPath != "" {
		return c.binPath
	}
	return c.cfg.FnpackBin
}

// Probe 探测 fnpack 是否可用并取版本号。结果会被缓存。
func (c *Client) Probe(ctx context.Context) (bin, version, message string) {
	c.mu.Lock()
	if c.probeResult != "" {
		bin, version, message = c.binPath, c.version, c.probeResult
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	bin = c.cfg.FnpackBin
	resolved, err := exec.LookPath(bin)
	if err != nil {
		message = fmt.Sprintf("找不到 fnpack（%s）：%v", bin, err)
	} else {
		bin = resolved
		// fnpack 没有 --version，直接跑会打印用法，版本号在用法里。
		out, runErr := exec.CommandContext(ctx, bin).CombinedOutput()
		if v := versionRe.FindStringSubmatch(string(out)); len(v) == 2 {
			version = v[1]
		}
		if runErr != nil && version == "" {
			message = fmt.Sprintf("执行 fnpack 失败：%v", runErr)
		} else if version == "" {
			message = "fnpack 已找到，但未能解析出版本号"
		}
	}

	c.mu.Lock()
	c.binPath, c.version, c.probeResult = bin, version, message
	c.mu.Unlock()
	return bin, version, message
}

// Run 在 dir 下执行 fnpack 子命令，输出逐行转给日志器。
func (c *Client) Run(ctx context.Context, dir string, source string, args ...string) (string, error) {
	bin := c.Bin()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// 让 fnpack 的临时目录落在数据目录内，避免跨挂载点 / 受限 /tmp。
	cmd.Env = c.cfg.Env()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	c.log.Line(source, logging.LevelInfo, fmt.Sprintf("$ fnpack %s", strings.Join(args, " ")))
	c.log.Line(source, logging.LevelDebug, fmt.Sprintf("  工作目录: %s", dir))

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动 fnpack 失败: %w", err)
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
		sb strings.Builder
	)
	collect := func(r *bufio.Scanner) {
		defer wg.Done()
		// 默认行长上限是 64KB。fnpack（或它调用的工具）偶尔打印超长行时，Scanner 会直接
		// 报错并停止扫描，而这里原本不查 Err()——日志与打包结论会静默截断，用户只能看到
		// 「怎么少了一段」。放宽上限，真读不下去时留一条记录。
		r.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for r.Scan() {
			line := r.Text()
			mu.Lock()
			sb.WriteString(line)
			sb.WriteByte('\n')
			mu.Unlock()
			c.log.Line(source, logging.LevelInfo, line)
		}
		if err := r.Err(); err != nil {
			c.log.Line(source, logging.LevelWarn, fmt.Sprintf("读取输出中断: %v", err))
		}
	}
	wg.Add(2)
	go collect(bufio.NewScanner(stdout))
	go collect(bufio.NewScanner(stderr))

	waitErr := cmd.Wait()
	wg.Wait()

	output := sb.String()
	if waitErr != nil {
		c.log.Line(source, logging.LevelError, fmt.Sprintf("fnpack 退出异常: %v", waitErr))
	}
	return output, waitErr
}

// Create 创建项目骨架。dir 是父目录，name 是项目名。
//
// 参数校验由 fnpack 负责：项目名必须是 3–32 位、以字母或数字开头结尾、中间可含
// . _ -。校验不过时它不会创建任何东西，也不会改退出码，只打印一行错误。
func (c *Client) Create(ctx context.Context, dir, name string, docker, withUI bool) (string, error) {
	args := []string{"create", name}
	if docker {
		args = append(args, "--template", "docker")
	}
	if !withUI {
		args = append(args, "--without-ui")
	}
	return c.Run(ctx, dir, logging.SourceFnpack, args...)
}

// Build 在项目目录内打包。
func (c *Client) Build(ctx context.Context, projectDir string) (string, error) {
	return c.Run(ctx, projectDir, logging.SourceBuild, "build")
}
