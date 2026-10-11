// Command fpkstudio 是飞牛 fpk 打包工具的可视化前端。
//
// 特点：
//   - 只监听 unix socket，不监听任何 TCP 端口
//   - 前端静态资源用 embed 打进二进制，单文件交付
//   - 所有路径由环境变量给出，非 root 运行
//
// 环境变量（完整列表见 README）：
//
//	FPKSTUDIO_DATA_DIR    数据目录（projects / logs / run / tmp / state.json）
//	FPKSTUDIO_OUTPUT_DIR  打包输出目录
//	FPKSTUDIO_SOCKET      unix socket 路径
//	FPKSTUDIO_BASE_URL    访问前缀，例如 /app/fpkstudio
//	FPKSTUDIO_FNPACK      fnpack 可执行文件
//	FPKSTUDIO_LOG_LEVEL   日志级别
//	FPKSTUDIO_TRIM_SOCKET 飞牛开放 API 的 socket（默认由系统提供）
//	FPKSTUDIO_TRIM_APP    应用名（开放 API 请求体里的 appName）
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"fpkstudio/internal/config"
	"fpkstudio/internal/fnpack"
	"fpkstudio/internal/logging"
	"fpkstudio/internal/projects"
	"fpkstudio/internal/server"
	"fpkstudio/internal/state"
	"fpkstudio/internal/trimapi"
)

// assets 是构建期由 Makefile 填进来的前端产物。
//
// 目录 backend/embed 不纳入版本管理：Makefile 先把 frontend/dist 复制进去再编译，
// 编译完立刻删掉。这样仓库里只有源码，二进制里却带着完整前端。
//
//go:embed all:embed
var assets embed.FS

// version 由构建脚本通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	var (
		flagVersion = flag.Bool("version", false, "打印版本号并退出")
		flagPaths   = flag.Bool("paths", false, "打印解析后的路径配置并退出")
	)
	flag.Parse()

	if *flagVersion {
		fmt.Printf("fpkstudio %s\n", version)
		return
	}

	cfg, err := config.Load(version)
	if err != nil {
		fatal(err)
	}

	if *flagPaths {
		fmt.Printf("数据目录    : %s\n", cfg.DataDir)
		fmt.Printf("项目目录    : %s\n", cfg.ProjectsDir)
		fmt.Printf("输出目录    : %s\n", cfg.OutputDir)
		fmt.Printf("临时目录    : %s\n", cfg.TmpDir)
		fmt.Printf("日志文件    : %s\n", cfg.LogFile())
		fmt.Printf("PID 文件    : %s\n", cfg.PIDFile)
		fmt.Printf("socket      : %s\n", cfg.SocketPath)
		fmt.Printf("base url    : %s\n", cfg.BaseURL)
		fmt.Printf("fnpack      : %s\n", cfg.FnpackBin)
		fmt.Printf("状态文件    : %s\n", cfg.StateFile)
		fmt.Printf("飞牛 socket : %s（appName=%s）\n", cfg.TrimSocket, cfg.TrimApp)
		return
	}

	log, err := logging.New(cfg.LogFile(), cfg.LogLevel, cfg.LogRing)
	if err != nil {
		fatal(fmt.Errorf("初始化日志失败: %w", err))
	}
	defer log.Close()

	fn := fnpack.New(cfg, log)

	prj, err := projects.New(cfg, log, fn)
	if err != nil {
		fatal(err)
	}
	defer prj.Close()

	// 目录设置存在数据目录下的 state.json，启动时应用一次：项目根目录与打包
	// 输出目录都可以被用户指向授权目录。打不开就沿用默认目录，不能让一个过期
	// 的路径把整个服务拖住起不来。
	st, err := state.Open(cfg.StateFile)
	if err != nil {
		log.Warnf("读取 %s 失败，目录设置按默认处理: %v", cfg.StateFile, err)
		st = state.Empty(cfg.StateFile)
	}
	if dirs := st.Dirs(); !dirs.Empty() {
		if err := prj.SetRoots(dirs.ProjectRoot, dirs.OutputDir); err != nil {
			log.Warnf("应用保存的目录设置失败，沿用默认目录: %v", err)
		} else {
			log.Infof("已应用保存的目录设置：项目根目录 %s，输出目录 %s", prj.ProjectRoot(), prj.OutputDir())
		}
	}

	// 飞牛开放 API（文件授权、权限检查、路径转换）只能服务端走 unix socket 调，
	// 前端要什么经 /api/trim/* 转一手。
	trim := trimapi.New(cfg.TrimSocket, cfg.TrimApp, log)

	// 静态资源在 embed/ 下，取出来作为资源根交给 HTTP 层。
	webFS, err := fs.Sub(assets, "embed")
	if err != nil {
		fatal(fmt.Errorf("内嵌的前端资源缺失（embed/）：%w；请用 make 构建", err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := server.New(cfg, log, fn, prj, webFS, st, trim)
	if err := srv.Run(ctx); err != nil {
		log.Errorf("服务异常退出: %v", err)
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "fpkstudio:", err)
	os.Exit(1)
}
