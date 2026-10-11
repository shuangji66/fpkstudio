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
| 编辑界面 | 每个文件一个弹窗 | 标签页：首页固定 + 每项目一个标签 + 项目内固定子标签（总览 / 快捷设置 / 图标）+ 文件子标签 |

## 环境变量

所有路径都由环境变量给出，程序不假设任何部署形态。日志、PID、socket、临时文件统一落在**数据目录**下。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `FPKSTUDIO_DATA_DIR` | `$XDG_DATA_HOME/fpkstudio` 或 `~/.local/share/fpkstudio` | 数据根目录 |
| `FPKSTUDIO_PROJECTS_DIR` | `$FPKSTUDIO_DATA_DIR/projects` | 项目根目录（项目列表与文件树的沙箱根），可以在数据目录之外 |
| `FPKSTUDIO_OUTPUT_DIR` | `$FPKSTUDIO_DATA_DIR/output` | 打包产物目录，可以在数据目录之外 |
| `FPKSTUDIO_SOCKET` | `$FPKSTUDIO_DATA_DIR/run/fpkstudio.sock` | unix socket 路径（上限 100 字节，超了会明确报错） |
| `FPKSTUDIO_BASE_URL` | `/` | 访问前缀，例如 `/app/fpkstudio` |
| `FPKSTUDIO_FNPACK` | `fnpack`（走 PATH） | fnpack 可执行文件 |
| `FPKSTUDIO_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `FPKSTUDIO_LOG_RING` | `5000` | 内存里保留的日志条数 |
| `FPKSTUDIO_TRIM_SOCKET` | `/var/run/trim_open_gateway_apiscope.socket` | 飞牛开放 API 的 socket（由系统提供，覆盖它只是为了在没有 fnOS 的环境里调链路） |
| `FPKSTUDIO_TRIM_APP` | `fpkstudio` | 开放 API 请求体里的 `appName`，要和 fpk 的 `manifest.appname` 一致 |

`FPKSTUDIO_PROJECTS_DIR` 与 `FPKSTUDIO_OUTPUT_DIR` 都只是**默认值**：两者都能在界面的
设置弹窗里改成用户授权的目录（见 [飞牛开放 API 接入](#飞牛开放-api-接入)），改过之后就
记在数据目录的 `state.json` 里，环境变量只在没设置时生效——也就是说优先级是
**界面设置的授权目录 > 环境变量 > 数据目录下的默认值**。

两个环境变量指到**同一个路径**时保项目目录：`FPKSTUDIO_OUTPUT_DIR` 按「没设置」处理、
输出目录退回 `$FPKSTUDIO_DATA_DIR/output`，设置弹窗里会标注它被忽略了。两个目录混在
一起的话，产物既算项目产物又算输出产物（列表里同一个文件出现两次）、复制还等于复制到
自己身上，谁都说不清它到底在哪。

界面上不允许把两个槽设成同一个目录（直接报错）。默认值这一侧还有一层兜底：项目目录可以
被指到数据目录下的任何位置，包括 `output/`——那正是输出目录的默认值，退回之后还是同一个。
这时**不拒绝启动**，按「没有单独的输出目录」处理（不重复复制、产物只按项目目录列一次），
因为把部署配置判成错误只会让服务起不来，而实际后果只是多余的复制。

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
├── state.json           用户在设置里改过的目录（项目根目录 / fpk 输出目录）
├── run/fpkstudio.sock
├── run/fpkstudio.pid
└── tmp/                 暂存与 fnpack 的 TMPDIR
```

`projects/` 与 `output/` 只在没人改过目录设置时才是项目 / 产物的实际位置。
项目根目录可以被指到数据目录之外（用户从飞牛文件选择器授权进来的目录），那时
`projects/` 就只是个空壳。

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
cd frontend && npm run check         # 语言包 + 向导回写 + 取景框几何 + 命名（四个检查脚本）
cd frontend && npm run dev           # 只起 Vite 开发服务器
```

关于 `npm run dev`：**它连不到后端**。后端只监听 unix socket，浏览器的 fetch
打不过去，所以 `dev` 只适合调纯 UI（布局、样式、组件）。要连后端调试，
走 `make && ./fpkstudio`，再用 nginx 把某个 location 反代到 socket（见上一节），
这样拿到的是完整链路。想临时桥一下直接在浏览器里点，`socat` 更省事：

```bash
FPKSTUDIO_DATA_DIR=/tmp/fpkstudio-data FPKSTUDIO_FNPACK=/bin/true ./fpkstudio &
socat TCP-LISTEN:18080,fork,reuseaddr UNIX-CONNECT:/tmp/fpkstudio-data/run/fpkstudio.sock &
```

没有 fnpack 也能验界面：直接在 `$FPKSTUDIO_DATA_DIR/projects/` 下手搓一个项目目录即可
（最少要有 `manifest`，且 `appname` / `version` / `platform` 三个键得齐）。

**给 AI / 自动化助手的操作手册在 [AGENTS.md](AGENTS.md)**：命令、目录、约定、
「已经踩过的坑」都收在那里，改代码前先读一遍。

## 飞牛开放 API 接入

应用跑在飞牛里时可以用系统的文件授权能力：让用户在应用自己的页面里选一个目录或文件，
系统把它的 ACL 授权给应用用户（应用服务以独立的应用用户运行，没有这条授权就读不到
用户的存储空间）。用到三个能力：

| Scope | 用来做什么 | 谁调 |
|---|---|---|
| `trim.file.userAccess` | 选目录 / 选文件并完成授权（`pickUserFile`）、查询与删除已授权目录 | 前端 JS SDK + 后端 |
| `trim.file.userAcl` | 导入前按当前用户检查目标文件是否可读 | 后端 |
| `trim.file.path` | 把 `/vol1/...` 转成用户看得懂的语义路径 | 后端 |

分工是硬性的，不是取舍：

- **前端**用 `@trimjs/web-app` 的 JS SDK 拉起选择器（这类能力必须有用户操作，且只能在
  宿主页面里调）。
- **后端**用开放 API（`POST /api/v1/trimapp`，只挂 unix socket）查授权、查权限、转路径。
  官方文档明确要求后端接口只能由服务端调、token 不能下发到浏览器，所以前端一律走
  `/api/trim/*` 转一手。

token 每次调用时从环境变量 `TRIM_API_TOKEN` 现读——系统在重新注册 / 重装 / 运行环境
变化后都会换新值，持久化下来只会拿到过期的那一个。当前用户取统一网关转发的
`X-Trim-Userid`（网关只在校验过登录态之后才转发这组 Header）；没经过网关时退化成后端
进程自己的 uid，单用户场景够用，也让这套接口在没有 fnOS 的环境里能跑通链路。

**这些能力要在 fpk 包的清单里声明才生效。** 这个仓库的交付形态是单文件二进制、没有应用包，
所以打包成 fpk 时要自己带上：

```
# manifest
micro_app=true
```

```json
// config/resource
{ "api-scope": ["trim.file.userAccess", "trim.file.userAcl", "trim.file.path"] }
```

`micro_app=true` 决定页面是否按微应用加载（不声明时 JS SDK 根本起不来）；声明
`api-scope` 只是接入前提，具体能访问哪个目录仍由用户授权、管理员设置和 token 决定。
`config/resource` 的编辑器里已经内置了这几个授权项，勾上即可。

### 界面上用到的地方

- **首页右上角「设置」**：占视口 90% 高、垂直居中的弹窗，正文自己滚（标题与「关于」
  不跟着滚走），遮罩带一层背景模糊，弹窗打开时锁住页面滚动。内容是主题切换 +
  两个目录槽（项目根目录、fpk 输出目录）+ 关于。
  两个槽各只能设一个目录，且只能来自文件选择器的授权；每槽显示 `trim.file.path` 转出来的
  展示路径（**转换失败才回落显示 `/vol1/...` 原始路径**，并另给一句失败原因），界面上不展示
  ACL 标签；每槽右侧有「选择 / 更换目录」和文件夹按钮（在文件管理器里打开该目录）。
  「取消设置」会删掉该目录的授权，并把这一槽还给环境变量给的默认目录。
- **换目录的顺序是有讲究的**：先完成新目录的授权与切换（配置落盘、后端切过去），
  **再**删旧目录的授权。反过来的话，用户一取消选择、或者选择器直接报错，旧授权已经没了
  而配置里还写着它，应用会突然读不到自己的项目目录。删旧授权失败只记一条警告——
  它留在系统里只是不干净，不该挡住换目录这件事。
- **语义路径失败要说出来**。`trim.file.convertPath` 的返回结构**与文档不一致**：文档写
  `{"status":0,"result":[…]}`，实测的系统直接回 `[{path,semanticPath}…]`。所以
  `internal/trimapi` 三种形态都认（还有「只回一串展示路径」的兜底），`data.status` 非 0
  也算失败；`language` 是必填项且走请求体（查询串会被反代丢掉）。失败时弹窗给一句原因，
  而不是默默退回显示 `/vol1/...`——「什么都没显示」是最难查的表现。
- **产物可以直接在文件管理器里打开**：产物行右侧的文件夹按钮调 SDK 的
  `openFileManager`（官方「页面路由」，无 scope 要求），目录由产物自己的绝对路径算出，
  不依赖 `/api/system/info` 里的 `outputDir`（换过目录之后它是旧的）。
- **用户取消选择不算错误**：宿主机在用户直接关掉选择器时返回 `code: 1` +
  `msg: "Operation failed"`，没有业务码也没有可读原因。`trim/sdk.ts` 把它与「成功但
  没返回路径」一起归一成空数组，界面给一句「已取消目录授权 / 已取消导入」，而不是弹
  一句「调用飞牛接口失败（1）」。
- **项目根目录真的会生效**：换成授权目录之后，项目列表、文件树、创建 / 导入 / 打包都读写
  那个目录，数据目录里只留日志、暂存和配置（见「实现上值得说明的几处」里的跨沙箱复制）。
- **导入 fpk**：在「选择文件」旁边多了「选择飞牛文件」，走 `pickUserFile` 让用户在飞牛的
  存储空间里挑 `.fpk`，选完由后端按路径读文件导入。浏览器拿不到 `/vol1/...` 这种路径，
  选择器只负责把路径授权给应用。
- **独立浏览器页面**（没有宿主桥，例如 `socat` 桥出来自己点）里这些入口一律置灰并说明
  原因。官方推荐的另一条路是 `openAppAuth` + `redirectUri` 回调页，它要求应用挂在统一
  网关下、回调页与原页面同源，本项目没有这条链路，所以选择「明确告诉用户去飞牛里打开」，
  而不是给一个点了就报错的按钮。

### 深色模式

`html.dark` 是唯一的开关：Tailwind 的 `dark:` 变体（在 `style.css` 里用
`@custom-variant dark` 改成认这个类）与 Element Plus 的暗色变量挂在同一个类上，由
`stores/theme.ts` 统一切换。「跟随系统」在宿主里跟的是飞牛主题（`$on('os/theme')`），
在独立浏览器里跟的是 `prefers-color-scheme`，两条都会实时响应。主题记在 localStorage，
不进 `state.json`（它是界面偏好，不是部署配置）。

## 前端

Vue 3.5 + Pinia 4 + Tailwind 4 + TypeScript 6.0.2 + Vite 8，UI 组件用 Element Plus 2，
代码编辑器 CodeMirror 6，构建日志弹窗用 xterm 6，路由 vue-router 5，国际化 vue-i18n 11。

几个刻意的设计：

- **标签页而不是弹窗**：首页「项目管理器」是固定标签；每打开一个项目占一个标签；
  项目内的子标签分两组，前面是三个固定标签——**总览**（打包设置 + manifest 预览）、
  **快捷设置**、**图标**，后面是打开的文件。前三个钉在标签栏最左边、不参与横向滚动，
  打开的文件再多也不会把它们挤出去。所有编辑都在标签里完成，
  打开的文件全部保持挂载，切标签不会丢未保存的内容。
- **三个固定子标签**：
  - `总览`：多架构发布、打包结果、产物列表；右侧固定显示 manifest 预览——逐行取
    原文、按最长键补齐、等宽字体渲染成对齐的 `key = value`，`checksum` 不参与预览
    （每次打包都会变），长值折三行、点一下展开。预览跟着 manifest 编辑页的保存同步。
    这一页就是原来的「打包」页。打开「多架构发布」时会检查 `building/platform/`：
    缺 `arm` / `x86` 就补建（只建目录、不自动勾选，空目录打出来的包是空的），
    与创建 / 导入项目时预置的目录一致。
  - `快捷设置`：把最常改的文件做成快捷入口，点一下就在子标签里打开，已经打开的
    直接切过去。带桌面入口的项目才有「桌面入口」，docker 应用才有 docker-compose。
    文件不存在时（向导、生命周期脚本常常是空的）按空内容打开，工具栏显示「未创建」
    而不是「未保存」，编辑后保存才落盘、父目录随之建出来。
    图标与环境变量不在这里——前者有专门的图标页，后者跟着生命周期脚本走。
    几组卡片用多列布局流式排布（宽屏两列、窄屏一列），而不是网格对齐：
    几组长短差得远，网格会把短的那组下面留出一块空洞。
  - `图标`：进页面先把项目里已有的应用大图标载进取景框当底图（包根的 `ICON_256.PNG`，
    没有就退回入口目录的 `icon_256.png`），都没有就等导入。
    导入一张图片后，在取景框里拖到想要的位置、滚轮或滑块缩放（取景框是正方形，
    框内就是结果，圆角也用 CSS 实时预览），浏览器里用 canvas 按框裁剪、切圆角，
    再按目标尺寸导出 PNG，一次写进勾选的图标位置（`ICON_256.PNG` / `ICON.PNG` 在包根，
    `app/ui/images/icon_256.png` / `icon_64.png` 在入口目录，没有入口的项目就没有这两个）。
    一个项目可以有多个桌面入口，每个入口一套 `<前缀>_256.png` / `<前缀>_64.png`（默认入口
    的前缀是 `icon`）。所以进页面时会把 `app/ui/images/` 扫一遍，**已有的副入口自动列成一组**
    ——导入的多入口 fpk 全靠这一步才看得见它们；只认 64 / 256 两种分辨率，别的文件不参与
    （`utils/icon.ts` 的 `entryIconPrefixes`，`check:icon` 盯着它）。列表下面也能「添加副入口
    图标」：填一个前缀（比如 `niubi`）就多出对应的两个写入项，加一组时会把所有写入勾选清掉
    ——这次要写的是另一套图标，沿用上一组的勾选很容易顺手把默认入口的图标覆盖掉。
    每一行左边有缩略图，**没写过的那一行留空**（虚线空框），勾上写入即可补全。
    前缀走 `entryIconPath` 拼路径（只留文件名里安全的字符，带不出 `app/ui/images/`）。
    不带桌面入口的项目根本不去探 `app/ui/images/`——那个目录本来就不存在，探了只会拿回
    一条 `no such file or directory` 把日志弄脏（项目形态是异步读回来的，读回来是带入口的
    项目才补扫一次）。
    小图缩放默认 1/4，即大图 256 时小图正好 64。写入直接覆盖同名文件，
    写完后文件树、缩略图与图片预览都会刷新（URL 上带了 `assetRev`，不吃浏览器缓存）。
- **文件树（左栏）**：右键节点、或用行尾那个低调的「···」出菜单——新建 / 重命名 /
  复制 / 剪切 / 粘贴 / 下载 / 删除。复制和剪切**只记路径**（剪贴板），粘到哪个目录由你选：
  在目录上粘贴就是粘进去，在文件上粘贴就落到它的父目录。剪贴中的那一行在树里淡一半，
  粘完即清空；目标重名时复制自动加「副本」、**绝不覆盖**，剪切则直接拒绝（改名到一个
  已存在的路径上，轻则报错重则覆盖，不如让用户自己决定）。菜单贴近窗口右 / 下边缘时会
  自己收回来，不会被切掉。
- **主题与目录设置**：首页右上角的「设置」按钮打开弹窗，主题（浅色 / 深色 / 跟随系统）
  与两个授权目录槽都在这里，见 [飞牛开放 API 接入](#飞牛开放-api-接入)。
- **首页（项目管理器）**：顶部工具条——项目数、搜索、刷新 / 新建 / 导入，按钮和搜索框
  左对齐，右上角是显示方式切换（平铺 / 列表，记在 localStorage）。工具条固定在顶部，
  下面的项目区才是滚动容器：列表排布在宽屏上是横向滚动的多列（见 `style.css` 的
  `.list-columns`），跟着工具条一起滚就没法用了。
  - 两种排布**都是点卡片即打开项目**（原来平铺卡片上那个占满一行的「打开」按钮去掉了：
    卡片就这么点地方，一个大按钮只是把可点区域缩小）。卡片右上角是操作区：文件夹按钮
    （在文件管理器的项目目录里打开，走 JS SDK 的 `openFileManager`，独立浏览器页面里
    置灰）+「···」菜单（重命名 / 导出 / 删除）。操作区自己挡住点击，不会连带打开项目。
    卡片可键盘操作（Enter / 空格），项目目录的绝对路径由 `/api/projects` 的 `absPath`
    给出——换过项目根目录之后，`projectsDir + name` 是拼不出来的。
  - `平铺`：卡片自适应列宽。
  - `列表`：一行一个项目，宽度足够时按固定列宽分列，列内先填满
    窗口高度再开下一列、横向滚动；窄屏只剩一列时纵向滚动。
  - 删除一定要过一遍确认弹窗（目录直接删，没有回收站）；重命名改的是**项目目录名**
    （`manifest.appname` 不动），如果这个项目开着标签，会关掉旧标签、按新名字重开一个
    ——所以有未保存改动时弹窗里会先提醒。两个动作都是「先改磁盘、再处理标签、最后刷列表」：
    刷列表会换掉 `v-for` 的 key、把这张卡片卸载掉，反过来做事件就丢了。
- **编辑器按路径注册**：`src/components/editor/registry.ts` 把「路径正则 → 组件」
  集中管理，加一种文件类型只需要加一个组件和一行注册。已知的 fpk 配置
  （manifest / privilege / resource / app/ui/config / wizard / cmd）都有表单化编辑器，
  并且可以在「表单 ⇄ 源码」之间切换。
- **环境变量弹窗**：生命周期脚本（`cmd/*`）的编辑页在「还原」左边有一个「环境变量」
  按钮，列出安装器注入的 `TRIM_*` 变量（按用途分组）、manifest 里的每个键、以及向导里
  收集到的字段（不带 `TRIM_` 前缀），点变量名即可复制。做法沿用 fnpackup 的
  「环境变量」页，只是从独立弹窗改成了挂在脚本编辑器上。
- **向导编辑器**：字段类型覆盖 fnpack 的全部七种（text / password / radio / checkbox /
  select / switch / tips），校验规则支持一个字段挂多条（必填 / 范围 min-max / 长度 len /
  正则 pattern），新建步骤与字段会预填可用默认值（步骤标题、不重名的字段名、按类型给
  默认值），校验提示也有默认文案。每个表单项能上移 / 下移，顺序就是向导里的显示顺序
  （直接改内存模型，保存时按数组顺序回写）。辅助文本单独一行、会自己撑高（说明文字
  常常是一整段）。向导文件**还不存在**时进来会先铺好第一步（带一个示例字段）：
  新建 / 刚导入的项目里这几份向导往往一个都没有，否则打开是纯空白、还得先点一下
  「添加步骤」；已经有内容的文件（哪怕内容是 `[]`）不动。安装 / 更新 / 配置三份向导
  往往就是同一套表单，工具栏的「复制其他向导」可以整份覆盖过来（只改内存，保存后才落盘）。
  解析与回写放在 `src/utils/wizard.ts`，和 `manifest.ts` 一样单独成模块：
  不认识的键（fnOS 以后加的字段）原样保留，不会因为表单里没有对应控件就被删掉。
- **manifest 回写保持原样**：`src/utils/manifest.ts` 逐行记住原文件（含注释、
  空行、对齐风格），只改动真正被编辑的键；`"""多行值"""` 会被正确识别。
  官方 fnpack 的模板是对过齐的，直接按行重排会让 diff 没法看。
  表单里「清空 = 不声明」这条语义由 `compact` 落实：安装位置选「存储位置」时
  `install_type` 会被删掉（而不是留一个空值），选「系统分区」写 `install_type=root`。
  链接（maintainer_url / distributor_url）和更新说明（changelog）跟描述一样是会自己长高的
  多行框——它们本来就可能很长，单行输入框只会把内容藏起来。
- **同一份产物挂任意 baseURL**：构建时 `base: './'`，服务端返回 index.html 时
  注入 `<base href>` 和 `window.__FPKSTUDIO__`。换挂载点不需要重新构建。
- **五个检查脚本**（`npm run check`，需要 Node ≥ 23.6，用到默认开启的类型剥离）：
  - `check:i18n`：两份语言包的每条消息都能被 vue-i18n 编译、键完全一致。消息里的 `{`
    是插值语法，一句带 `${...}` 的提示会让这条消息在 `t()` 时抛错——而且是在渲染到
    那个组件时才抛，表现是「弹窗整体画不出来」，所以让它在这一步就被挡住。
  - `check:wizard`：`utils/wizard.ts` 的解析 / 回写往返（真实 fpk 向导、七种字段类型、
    四种校验、不认识的键要保留）。这段逻辑会重写用户项目里的文件，值得盯住。
  - `check:icon`：取景框的几何——初始铺满居中、拖到任何位置都不露空白、以指针为锚
    缩放、取景框对应的原图矩形始终在图片范围内，以及裁剪 / 圆角 / 导出的参数；
    另外盯住副入口图标的两件事：前缀不会跑出 `app/ui/images/`（前缀直接拼进文件名），
    以及目录里已有的 `<前缀>_64/256.png` 能不能被正确认成副入口（只认 64 / 256、
    排掉默认入口 `icon`、只有一个尺寸也算）。
  - `check:format`：`utils/format.ts` 的 `uniqueName`（粘贴时挑不重名的那个名字）。
    它算错的表现是「粘贴把已有文件覆盖掉」，所以值得单独盯住。
  - `check:semantic`：`utils/semantic.ts` 把「请求了哪几条路径」和 `trim.file.convertPath`
    返回的条目对上号（尾斜杠归一、顺序一致时按位置兜底）。它错了的表现是设置弹窗里
    「语义路径不显示、退回 /vol1/...」，而且不报错，所以值得单独盯住。

## HTTP API

所有接口都在 `${BASE_URL}api/` 下。`path` 参数一律相对**项目根目录**。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/system/info` | 版本、目录、socket、fnpack 探测结果 |
| GET | `/api/projects` | 项目列表 |
| POST | `/api/projects/create` | `{name, docker, ui}` → 调 `fnpack create`；项目名需 3–32 位、字母数字开头结尾，中间可含 `. _ -` |
| POST | `/api/projects/delete` | `{name}` |
| GET | `/api/projects/exists?name=` | 是否存在、是否 docker、是否有桌面入口 |
| GET | `/api/projects/platforms?name=` | `building/platform/` 下的架构目录 |
| GET/PUT | `/api/projects/settings?name=` | 打包偏好（`platforms` 始终是数组，不会返回 `null`） |
| POST | `/api/projects/import` | multipart 上传 `.fpk`，解包成项目 |
| POST | `/api/projects/pack` | `{name, platforms[], serverDir, copyOut}` |
| GET | `/api/projects/artifacts?name=` | 产物列表（项目内 + 输出目录） |
| GET/POST | `/api/fs/list?path=` | 目录列表，`path` 也可以放在 JSON body 里（前端就是这么调的） |
| GET/POST | `/api/fs/read?path=` | 读文件（二进制会返回 `text:false`，不带内容） |
| PUT | `/api/fs/write` | `{path, content}` |
| POST | `/api/fs/create` \| `delete` \| `rename` \| `copy` | 文件操作 |
| POST | `/api/fs/upload` | multipart，`app/server/` 下的文件按 0755 落地 |
| GET | `/api/fs/download?path=` | 下载文件，目录会打成 zip |
| GET | `/api/logs?after=&limit=&source=` | 增量拉日志 |
| GET | `/api/logs/stream?after=` | SSE 实时日志 |
| GET/PUT | `/api/settings` | 目录设置（`{projectRoot, outputDir}`，空字符串 = 回默认）；GET 还会带上当前用户与飞牛接口可用状态 |
| GET | `/api/trim/status` | 飞牛开放 API 的可用状态 + 当前用户 |
| GET | `/api/trim/folders` | 当前用户授权给本应用的目录 |
| POST | `/api/trim/folders/delete` | `{path}` → 删除一条目录授权 |
| POST | `/api/trim/acl` | `{paths[]}` → 当前用户对若干路径的读 / 写 / 删权限 |
| POST | `/api/trim/semantic` | `{paths[]}` + `?language=` → 语义化路径（`/vol1/...` → 存储空间1/…） |
| POST | `/api/projects/import-path` | `{path}` → 导入本机某个绝对路径上的 fpk（飞牛文件选择器选的） |

`/api/trim/*` 与 `/api/projects/import-path` 都按**飞牛统一网关转发的用户**判权限
（`X-Trim-Userid`，见下节），没经过网关时退化成后端进程自己的 uid。

## 目录结构

```
.
├── Makefile                    一条 make 出二进制
├── AGENTS.md                   给 AI / 自动化助手看的操作手册（命令、约定、坑）
├── backend/                    Go 后端（module 根）
│   ├── main.go                 入口：embed、配置、日志、依赖装配、信号处理
│   ├── embed/                  构建期临时目录：前端产物放这里再 //go:embed，编译后即删
│   └── internal/
│       ├── config/             环境变量解析、目录准备、socket 长度校验
│       ├── logging/            文件日志 + 内存环形缓冲 + 订阅（SSE 的数据源）
│       ├── fsx/                os.Root 之上的沙箱文件操作
│       ├── fnpack/             fnpack 调用与输出转发
│       ├── projects/           项目列表、创建、导入、多架构打包、产物、可切换的项目根
│       ├── state/              数据目录下 state.json 的读写（项目根 / 输出目录）
│       ├── trimapi/            飞牛开放 API 客户端（只走 unix socket 的服务端接口）
│       └── server/             HTTP 路由、静态资源、各项 API（含 /api/trim/* 的转发）
└── frontend/                   Vue 前端（Vite 产物到 frontend/dist）
    ├── src/
    │   └── trim/               飞牛 JS SDK 的封装（环境判断、选择器、宿主主题）
    └── dist/                   构建产物，被复制进 backend/embed
```

`backend/embed`、`frontend/dist`、`frontend/node_modules`、`./fpkstudio` 以及
Makefile 用的本地缓存目录都在 `.gitignore` 里，仓库里只留源码。

首页的项目操作（重命名 / 导出）没有专门的接口：重命名就是把项目目录当普通目录改名
（`/api/fs/rename`），导出就是下载这个目录（`/api/fs/download` 对目录打 zip）。
换句话说，项目目录本身就是项目，没有另一份元数据要同步——`manifest.appname` 是
应用标识，改目录名不会动它。

## 实现上值得说明的几处

- **多架构出包**。`building/platform/<架构>/` 存各架构的产物，打包时把它复制到
  `app/server`、把 `platform` 写进 manifest，每个架构出一个 fpk。包里带了架构相关的
  东西却写 `platform=all` 是最常见的翻车点——应用中心会把它装到不匹配的设备上然后起不来，
  所以编辑器在检测到「`platform=all` 且项目里已经有架构目录」时会给出提示。
  新建项目时预置 `arm` / `x86` 两个目录；导入的 fpk 里只有打包后的 `app/`，没有这套
  工作目录，所以导入后也补上（同一份 `platformDirs`），前端在打开「多架构发布」开关时
  再兜一次——补充见 `backend/internal/projects/import_test.go`。
- **不修改用户的项目文件**。fnpackup 是原地改 `manifest.platform`、覆盖 `app/server`
  再打包，打一次项目就脏了。这里把项目整体复制到 `tmp/build-*`，在副本上改、在副本上
  打包，结束删掉；`fsx` 的复制会把权限位显式 `chmod` 一遍。
- **权限位显式设置**。飞牛的存储卷上，新建文件会继承父目录的模式位、`open(O_CREAT, mode)`
  的 mode 参数被忽略；实测新建文件会落成 `0000`，这种文件打不进 fpk（fnpack 会在复制
  阶段 `EACCES` 失败）。所以写文件、复制、上传后都会显式 `chmod`。
- **`TMPDIR` 固定到数据目录**。fnpack 会先把整个包复制到临时目录再打包，
  `TMPDIR` 跨挂载点或落在受限的 `/tmp` 时会直接失败。
- **项目根目录可以被换成授权目录**。默认值来自环境变量，改过之后记在数据目录的
  `state.json`，启动时应用一次（打不开就沿用默认目录并记一条日志，不让一个过期的路径
  把服务拖住起不来）。项目根落在数据目录之外时，打包前的暂存复制跨了两个沙箱：单靠
  任一个 `fsx.Root` 的相对路径都表达不了，改走 `fsx.CopyTree` 的绝对路径复制；导入时
  从暂存区搬进项目根也同理，先试 `rename`（同文件系统瞬时完成），跨设备 `EXDEV` 时
  退回复制 + 删源。
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
- `wizard` 编辑器的表单不呈现「认识之外的键」（例如 fnOS 以后新增的字段），
  但它们会原样保留、保存时写回；界面上只列一下键名，避免误以为被删掉了。
- 向导的取值校验（范围 / 长度 / 正则）只是编辑器侧的字段约束，最终仍以 fnOS
  安装器的行为为准。
- UI 只做了中英双语，语言按浏览器 `navigator.language` 选择，没有做切换入口（主题倒是
  可以在设置里切）。
- 文件选择与授权类能力只在飞牛宿主页面里可用。用 `socat` / 反代直接打开时没有这些入口，
  相关按钮置灰并提示原因。
- 用户身份取网关转发的 `X-Trim-Userid`；直接调 socket（没有网关）时用的是后端进程自己的
  uid，多用户场景下查出来的授权与 ACL 不一定对。要做多用户，得按官方建议把应用挂到统一
  网关下。
- 目录设置改到授权目录之后，已有项目**不会自动迁移**：改回默认目录就是换一个地方找项目，
  文件还留在原来的位置。

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
