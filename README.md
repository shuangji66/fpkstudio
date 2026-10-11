# fpkstudio

飞牛 fnOS 的 fpk 打包工作台：在浏览器里浏览、编辑 fpk 项目目录，调用官方 `fnpack` 打包，支持多架构分别出包。

它是 [fnpackup](https://github.com/snltty/fnpackup) 的重写版本——保留了「把 fpk 包目录做成可视化编辑器」这个核心价值，去掉了静态托管，并把交付形态从
「Docker 容器 + 特权 + TCP 端口」换成了「单文件静态二进制 + 非 root + 只监听 unix socket」。

## 与 fnpackup 的主要差异

| | fnpackup | fpkstudio |
|---|---|---|
| 后端 | .NET 8，跑在容器里 | Go 1.27，单文件自包含二进制 |
| 前端嵌入 | 编译进单文件 | `go:embed`，同一份产物可挂任意 baseURL |
| 监听 | TCP 1069 / 10699 | **只有 unix socket**，不占任何 TCP 端口 |
| 鉴权 | 无（端口直连可绕过 CGI 登录态） | 无（只能由 socket 文件权限兜底） |
| 运行身份 | 容器内 root（`--privileged`） | 普通用户，不需要任何特权 |
| 路径 | 硬编码容器内路径（`/app/...`、`/usr/share/...`） | 全部来自环境变量 |
| 打包是否改项目文件 | 会（原地改 manifest、覆盖 app/server） | **不会**，在暂存副本里操作 |
| 静态托管 | 有 | 无 |
| 编辑界面 | 每个文件一个弹窗 | 标签页：首页固定 + 每项目一个标签 + 项目内文件子标签 |

## 环境变量

所有路径都由环境变量给出，程序不假设任何部署形态。日志、PID、socket、临时文件统一落在**数据目录**下。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `FPKSTUDIO_DATA_DIR` | `$XDG_DATA_HOME/fpkstudio` 或 `~/.local/share/fpkstudio` | 数据根目录 |
| `FPKSTUDIO_OUTPUT_DIR` | `$FPKSTUDIO_DATA_DIR/output` | 打包产物目录，可以在数据目录之外 |
| `FPKSTUDIO_SOCKET` | `$FPKSTUDIO_DATA_DIR/run/fpkstudio.sock` | unix socket 路径（上限 100 字节，超了会明确报错） |
| `FPKSTUDIO_BASE_URL` | `/` | 访问前缀，例如 `/app/fpkstudio` |
| `FPKSTUDIO_FNPACK` | `fnpack`（走 PATH） | fnpack 可执行文件 |
| `FPKSTUDIO_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `FPKSTUDIO_LOG_RING` | `5000` | 内存里保留的日志条数 |

### 数据目录布局

```
$FPKSTUDIO_DATA_DIR/
├── projects/            项目目录（fpk 包目录）
│   └── <appname>/
│       ├── manifest  config/  cmd/  wizard/  app/  i18n/
│       └── building/
│           ├── settings.json        打包偏好（架构选择、落地目录…）
│           └── platform/<架构>/      各架构的产物，打包时复制到 app/server
├── output/              打包产物（FPKSTUDIO_OUTPUT_DIR）
├── logs/fpkstudio.log   运行日志（滚动 8MB × 3）
├── run/fpkstudio.sock
├── run/fpkstudio.pid
└── tmp/                 暂存与 fnpack 的 TMPDIR
```

## 快速开始

一条 `make` 就够了：构建前端 → 复制进 `backend/embed` → 编译后端 → 清掉 `embed`，
最终在**项目根目录**得到 `./fpkstudio`。

```bash
make                    # 默认 dev 构建（不 strip），版本 1.0.0
make release V=1.0.1    # release 构建（strip 符号），版本可覆盖
make test               # 后端单元测试
make clean              # 删除二进制与 embed 目录
```

```bash
# 运行
FPKSTUDIO_DATA_DIR=/srv/fpkstudio \
FPKSTUDIO_BASE_URL=/app/fpkstudio \
FPKSTUDIO_FNPACK=/usr/local/bin/fnpack \
./fpkstudio
```

启动后会打印实际生效的路径：

```
fpkstudio 1.0.0 已启动
socket   : unix:/srv/fpkstudio/run/fpkstudio.sock
base url : /app/fpkstudio
数据目录 : /srv/fpkstudio
输出目录 : /srv/fpkstudio/output
示例     : curl --unix-socket /srv/fpkstudio/run/fpkstudio.sock /app/fpkstudio/api/system/info
```

查看解析后的配置而不启动服务：

```bash
./fpkstudio -paths
```

## 访问方式

只监听 unix socket，没有「打开端口就能用」这一说。两种接法：

**1. 直接调 API**

```bash
curl --unix-socket /srv/fpkstudio/run/fpkstudio.sock \
     http://localhost/app/fpkstudio/api/system/info
```

**2. 用 nginx / Caddy 反代给浏览器用**

```nginx
location /app/fpkstudio/ {
    proxy_pass http://unix:/srv/fpkstudio/run/fpkstudio.sock;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_buffering off;   # 构建日志走 SSE，必须关缓冲
}
```

`FPKSTUDIO_BASE_URL` 要和反代的 location 对上（上面这个例子就是 `/app/fpkstudio`）。
同一份前端产物可以挂任意前缀：服务端返回 index.html 时会按实际前缀注入 `<base href>`
和运行时配置，换挂载点不需要重新构建。

> 应用自身不做鉴权（按需求）。**唯一的访问控制就是 socket 文件的权限（0660）**，
> 以及你套在外面的反代。不要把 socket 暴露给不可信的用户或进程。

## 开发

```bash
make          # 完整构建，产出 ./fpkstudio
make test     # go test ./...
make clean    # 清产物
```

`make` 之外还有两个常用入口：

```bash
cd backend  && go vet ./...          # 后端静态检查（需要 GOCACHE/TMPDIR，见下）
cd frontend && npm run typecheck     # 前端类型检查
cd frontend && npm run dev           # 只起 Vite 开发服务器
```

关于 `npm run dev`：**它连不到后端**。后端只监听 unix socket，浏览器的 fetch
打不过去，所以 `dev` 只适合调纯 UI（布局、样式、组件）。要连后端调试，
走 `make && ./fpkstudio`，再用 nginx 把某个 location 反代到 socket（见上一节），
这样拿到的是完整链路。

## 前端

Vue 3.5 + Pinia 4 + Tailwind 4 + TypeScript 6.0.2 + Vite 8，UI 组件用 Element Plus 2，
代码编辑器 CodeMirror 6，构建输出控制台用 xterm 6，路由 vue-router 5，国际化 vue-i18n 11。

几个刻意的设计：

- **标签页而不是弹窗**：首页「项目管理器」是固定标签；每打开一个项目占一个标签；
  项目内再分文件子标签（外加固定的「打包」标签）。所有编辑都在标签里完成，
  打开的文件全部保持挂载，切标签不会丢未保存的内容。
- **编辑器按路径注册**：`src/components/editor/registry.ts` 把「路径正则 → 组件」
  集中管理，加一种文件类型只需要加一个组件和一行注册。已知的 fpk 配置
  （manifest / privilege / resource / app/ui/config / wizard / cmd）都有表单化编辑器，
  并且可以在「表单 ⇄ 源码」之间切换。
- **manifest 回写保持原样**：`src/utils/manifest.ts` 逐行记住原文件（含注释、
  空行、对齐风格），只改动真正被编辑的键；`"""多行值"""` 会被正确识别。
  官方 fnpack 的模板是对齐过的，直接按行重排会让 diff 没法看。
- **同一份产物挂任意 baseURL**：构建时 `base: './'`，服务端返回 index.html 时
  注入 `<base href>` 和 `window.__FPKSTUDIO__`。换挂载点不需要重新构建。

## HTTP API

所有接口都在 `${BASE_URL}api/` 下。`path` 参数一律相对**项目根目录**。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/system/info` | 版本、目录、socket、fnpack 探测结果 |
| GET | `/api/projects` | 项目列表 |
| POST | `/api/projects/create` | `{name, docker, ui}` → 调 `fnpack create` |
| POST | `/api/projects/delete` | `{name}` |
| GET | `/api/projects/exists?name=` | 是否存在、是否 docker、是否有桌面入口 |
| GET | `/api/projects/platforms?name=` | `building/platform/` 下的架构目录 |
| GET/PUT | `/api/projects/settings?name=` | 打包偏好 |
| POST | `/api/projects/import` | multipart 上传 `.fpk`，解包成项目 |
| POST | `/api/projects/pack` | `{name, platforms[], serverDir, copyOut}` |
| GET | `/api/projects/artifacts?name=` | 产物列表（项目内 + 输出目录） |
| GET | `/api/fs/list?path=` | 目录列表 |
| GET | `/api/fs/read?path=` | 读文件（二进制会返回 `text:false`，不带内容） |
| PUT | `/api/fs/write` | `{path, content}` |
| POST | `/api/fs/create` \| `delete` \| `rename` \| `copy` | 文件操作 |
| POST | `/api/fs/upload` | multipart，`app/server/` 下的文件按 0755 落地 |
| GET | `/api/fs/download?path=` | 下载文件，目录会打成 zip |
| GET | `/api/logs?after=&limit=&source=` | 增量拉日志 |
| GET | `/api/logs/stream?after=` | SSE 实时日志 |

## 目录结构

```
.
├── Makefile                    一条 make 出二进制
├── backend/                    Go 后端（module 根）
│   ├── main.go                 入口：embed、配置、日志、依赖装配、信号处理
│   ├── embed/                  构建期临时目录：前端产物放这里再 //go:embed，编译后即删
│   └── internal/
│       ├── config/             环境变量解析、目录准备、socket 长度校验
│       ├── logging/            文件日志 + 内存环形缓冲 + 订阅（SSE 的数据源）
│       ├── fsx/                os.Root 之上的沙箱文件操作
│       ├── fnpack/             fnpack 调用与输出转发
│       ├── projects/           项目列表、创建、导入、多架构打包、产物
│       └── server/             HTTP 路由、静态资源、各项 API
└── frontend/                   Vue 前端（Vite 产物到 frontend/dist）
    ├── src/
    └── dist/                   构建产物，被复制进 backend/embed
```

`backend/embed`、`frontend/dist`、`frontend/node_modules`、`./fpkstudio` 以及
Makefile 用的本地缓存目录都在 `.gitignore` 里，仓库里只留源码。

## 实现上值得说明的几处

- **多架构出包**。`building/platform/<架构>/` 存各架构的产物，打包时把它复制到
  `app/server`、把 `platform` 写进 manifest，每个架构出一个 fpk。包里带了架构相关的
  东西却写 `platform=all` 是最常见的翻车点——应用中心会把它装到不匹配的设备上然后起不来，
  所以编辑器在检测到「`platform=all` 且项目里已经有架构目录」时会给出提示。
- **不修改用户的项目文件**。fnpackup 是原地改 `manifest.platform`、覆盖 `app/server`
  再打包，打一次项目就脏了。这里把项目整体复制到 `tmp/build-*`，在副本上改、在副本上
  打包，结束删掉；`fsx` 的复制会把权限位显式 `chmod` 一遍。
- **权限位显式设置**。飞牛的存储卷上，新建文件会继承父目录的模式位、`open(O_CREAT, mode)`
  的 mode 参数被忽略；实测新建文件会落成 `0000`，这种文件打不进 fpk（fnpack 会在复制
  阶段 `EACCES` 失败）。所以写文件、复制、上传后都会显式 `chmod`。
- **`TMPDIR` 固定到数据目录**。fnpack 会先把整个包复制到临时目录再打包，
  `TMPDIR` 跨挂载点或落在受限的 `/tmp` 时会直接失败。
- **`os.Root` 做沙箱**。所有文件操作都在 `os.Root` 内进行，逃逸（`../`、指向外部的
  符号链接）在 `openat` 层面被拒绝，不依赖字符串前缀检查。
- **单实例保护**。启动时先试着连一下 socket：连得上说明已有实例在跑，直接拒绝启动；
  连不上才当残留文件清掉。退出时只删自己那个 socket（比对 inode），不会误删新实例的。
- **构建产物清理**。fnpack 模板自带 `.DS_Store`，在部分文件系统上会让 `fnpack build`
  直接失败，所以创建和导入项目后都会清掉这些文件。

## 已知限制

- 没有鉴权（按需求）。请依赖 socket 文件权限，以及你自己套在外面的反代。
- 打包是同步请求，超大项目会让这个 HTTP 请求长时间不返回；日志通过 SSE 实时输出，
  前端不会卡住，但请求本身没有超时控制。
- 没有项目级别的并发保护：同一个项目同时触发两次打包，第二次会等在暂存区复制之后，
  结果是两份产物互相覆盖（不会污染项目本身）。
- `wizard` 编辑器覆盖常见字段（text / tips / radio / switch / checkbox +
  `rules[0].required`），不支持的字段会原样保留但不在表单里呈现。
- UI 只做了中英双语，语言按浏览器 `navigator.language` 选择，没有做切换入口。

## 构建环境说明

Makefile 把构建期需要的环境变量都收敛到项目本地目录，`make` 不需要你额外设置任何东西：

| 变量 | 默认值 | 为什么需要 |
|---|---|---|
| `GOCACHE` | `./.gocache` | 有些环境里 `$HOME` 下的缓存不可写，`go build` 会报 `package fmt is not in std` 这类误导性错误 |
| `GOPATH` | `./.gopath` | 同上 |
| `TMPDIR` | `./.tmp` | `go build` 的 `$WORK` 走 `os.TempDir()`；`/tmp` 受限时会报 `open /tmp/go-build.../exe/a.out: permission denied` |
| `CGO_ENABLED` | `0` | 纯 Go 程序不需要 cgo；关掉后产物静态链接，而且冷编译不再走 `runtime/cgo`（那一步会写 `.o` 中间文件，在某些存储卷上会被损坏） |
| `GOFLAGS` | `-buildvcs=false` | 仓库可能没有 `.git`，不必因此失败 |
| `PATH` | 前置 `/var/apps/nodejs_v*/target/bin` | fnOS 上 Node 由应用提供，不一定在 `PATH` 里；取版本号最高的一个，可用 `NODE_BIN=` 覆盖 |

这些都是 `?=` 或追加式赋值，**环境里已有的值优先**，所以放进 CI 或别的构建机也能用。

两个只在特定文件系统上才会踩到的坑，Makefile 里也处理了：

- 复制前端产物用的是 `tar` 管道而不是 `cp -r`。飞牛的存储卷不兑现 `mkdir` 请求的
  模式位，`cp` 生成的目录会落成 `0000` 且丢掉继承来的 ACL，`go:embed` 直接报
  `pattern all:embed: permission denied`；`tar` 解包后会显式 `chmod` 目录，结果才可用。
- 编译后显式 `chmod 755` 二进制。同一个原因——新建文件的模式位不可靠，
  不补这一下拿到的是个跑不起来的 `0000` 文件。

## 许可证

MIT
