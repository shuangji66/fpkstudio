# AGENTS.md

给在这个仓库里干活的 AI / 自动化助手看的操作手册。**先读这里，再读 README.md**
（README 讲「为什么这么设计」，这里讲「怎么改、改完怎么验」）。

## 这是什么

飞牛 fnOS 的 fpk 打包工作台：浏览器里浏览 / 编辑 fpk 包目录、调官方 `fnpack` 打包，
支持多架构分别出包。Go 后端（单文件、只监听 unix socket）+ Vue 3 前端（构建产物 `go:embed`
进二进制）。没有鉴权，访问控制靠 socket 文件权限。

## 命令

构建、测试命令都在 `Makefile` 里；`make` 需要的环境变量（GOCACHE/GOPATH/TMPDIR/
CGO_ENABLED/GOFLAGS/PATH）都由它自己收敛到项目本地目录，**不要**额外设置。

```bash
make                     # 完整构建：前端 → backend/embed → 编译 → 清 embed，产出 ./fpkstudio
make test                # 后端单元测试（会先补一个 embed 占位文件）
make clean               # 删二进制与 embed

cd frontend
npm run typecheck        # vue-tsc，类型检查（改了 .vue / .ts 必跑）
npm run check            # 五个检查脚本：i18n 键一致 + 向导往返 + 取景框几何/图标路径 + 命名 + 语义路径
npm run build            # vite 构建到 frontend/dist
```

改完前端至少要跑 `npm run typecheck && npm run check`；动了后端要跑 `make test`；
只跑一个不算验证完成。

`npm run dev` 起的是纯前端 dev server，**连不到后端**（后端只有 unix socket），
只适合调布局。要跑完整链路见下一节。

## 怎么真跑起来（端到端）

后端的 socket 浏览器够不着，临时桥一下即可：

```bash
make                                     # 或只构建二进制
FPKSTUDIO_DATA_DIR=/tmp/fpkstudio-data FPKSTUDIO_FNPACK=/bin/true ./fpkstudio &
socat TCP-LISTEN:18080,fork,reuseaddr UNIX-CONNECT:/tmp/fpkstudio-data/run/fpkstudio.sock &
# 浏览器打开 http://127.0.0.1:18080/
```

没有 fnpack 也能验界面：直接在 `$FPKSTUDIO_DATA_DIR/projects/` 下手搓一个项目目录即可，
最少要有 `manifest`（`appname` / `version` / `platform` 三个键，`/api/projects` 才认）。
`app/ui/config` 存在 = 带桌面入口，`app/docker/docker-compose.yaml` 存在 = docker 应用。

想验「导入」链路就造一个真的 fpk（gzip tar，内含 `manifest` + `app.tgz` 或 `app/`），
`unpackFPK` 两种形态都认。

## 目录

```
backend/
  main.go                    入口：embed、配置、日志、依赖装配
  internal/config/           环境变量解析、目录准备、socket 长度校验
                             （项目目录 / 输出目录撞一起时保项目目录、输出回默认）
  internal/logging/          文件日志 + 内存环 + 订阅（SSE 的数据源）
  internal/fsx/              os.Root 之上的沙箱文件操作（所有文件操作都走它）
  internal/fnpack/           fnpack 调用与输出转发
  internal/projects/         项目列表 / 创建 / 导入 / 多架构打包 / 产物 / manifest 解析
                             （项目根目录可以在数据目录之外，详见 SetRoots 的注释）
  internal/state/            数据目录下 state.json 的读写（项目根目录 / 输出目录）
  internal/trimapi/          飞牛开放 API 客户端：只能服务端走 unix socket 调
  internal/server/           HTTP 路由、静态资源、各 API（/api/trim/* 是开放 API 的转发）
frontend/src/
  api/                       endpoints（接口封装）、client（fetch 封装）、types
  stores/                    pinia：projects / tabs / workspace / system / logs / theme
  components/project/        项目内的固定页：总览 / 快捷设置 / 图标（+ 文件树、文件标签）
  components/editor/         标签页编辑器宿主 + 按路径注册的表单编辑器
  components/home/           首页：项目卡片 / 新建 / 导入
  utils/                     manifest、wizard、icon、format 等纯逻辑（可单独跑检查）
  i18n/                      zh-CN / en-US 两份语言包
  trim/                      飞牛 JS SDK 的封装（@trimjs/web-app）
```

## 约定（改代码前先看）

- **注释写中文**，解释「为什么」而不是「做了什么」。文件头部的模块注释是设计说明，
  改动行为时顺手更新它，别只改代码把注释留在旧语义上。
- **最小改动**。不做无关重构、不统一风格、不顺手改格式。已有模块的写法优先于「更漂亮」的写法。
- **不要动用户项目里的文件**。打包是把项目复制到 `tmp/build-*` 再改副本；任何「原地改」
  用户的 manifest / app 的行为都不要引入。
- **文件权限要显式设置**。飞牛的存储卷上 `O_CREAT` 的 mode 会被忽略，新建文件会落成 `0000`，
  这种文件打不进 fpk。新增写文件 / 复制 / 上传的代码路径时要跟 `fsx` 一样补 `chmod`。
- **所有文件操作走 `fsx.Root`**（`os.Root` 沙箱）。不要用 `os.Open` 直接拼路径。
- **语言包两份必须同步**。`npm run check:i18n` 会挡住键不一致；消息里的 `{` 是插值语法，
  想写字面量 `{` 要转义。一条消息写错，表现是「渲染到那个组件时整个弹窗画不出来」。
- **`utils/` 里的纯逻辑要能单独验**。解析 / 回写 / 几何这类会写坏用户文件或算错图的计算，
  放 `utils/` 并在 `frontend/scripts/check-*.mjs` 里加断言，别塞进组件里只靠手点。
- **前端只用 Composition API + `<script setup lang="ts">`**，SFC 段落顺序
  `<script>` → `<template>` → `<style>`。组件超过一个明确职责就拆。
- **UI 文案全部走 i18n**，不要在组件里硬编码中文（历史遗留：manifest 的
  `platform` / `source` / `install_type` 选项直接写在字段表里，改文案时注意两份语言包都要动）。
- **主题开关就是 `html.dark`**。Tailwind 的 `dark:` 变体（在 `style.css` 里用
  `@custom-variant dark` 改成认这个类）和 Element Plus 的暗色变量挂在一起，
  由 `stores/theme.ts` 统一切。**新写界面时每个浅色都配一个 `dark:` 变体**，
  漏掉的地方在深色下就是一块白斑，编译期不会报错。
- **飞牛开放 API 只由后端调**（`internal/trimapi`，token 现读 `TRIM_API_TOKEN`，身份取
  网关转发的 `X-Trim-Userid`）。前端一律走 `/api/trim/*`：token 和这套 socket 都不能
  暴露给浏览器。
- **前端调宿主能力前先判环境**。`trim/sdk.ts` 里 `isHostAvailable()` 为假（独立浏览器
  页面）时按钮要置灰并说明原因；`pickUserPaths` 还会先等 `ensureHostReady()`，
  理由见下面「已经踩过的坑」。

## 已经踩过的坑

- `fnpack create` 失败时**退出码仍是 0**，错误只打在 stdout 上——判据要看目录 / manifest
  有没有真的落地（见 `Manager.Create`）。
- `fnpack` 模板自带 `.DS_Store`，在部分文件系统上会让 `fnpack build` 直接失败，
  创建 / 导入项目后都要清一遍。
- `TMPDIR` 必须落在数据目录（`fnpack` 会先把整个包复制到临时目录），
  `/tmp` 受限时构建会失败。
- 标签页里的组件用 `v-show` 全部保持挂载：`innerText` 对隐藏面板返回
  `textContent` 一类的内容，用 DOM 断言时要注意选到「可见的那一个」。
- 只改 hash 的地址跳转**不会重新加载页面**：端到端验证时想拿干净状态要带上别的
  query（例如 `?fresh=1#/project/demo`），否则沿用上一个项目标签的内存状态。
- `position: fixed` 的元素 `offsetParent` 是 `null`：用 DOM 断言判断「菜单有没有显示」
  时要看 `getBoundingClientRect().width > 0`，别用 `offsetParent`。
- 列表里的每一项都带自己的 `el-dropdown` 时，弹层（即使隐藏着）**全都留在 DOM 里**：
  `document.querySelectorAll('.el-dropdown-menu__item')` 会同时命中别的卡片的菜单，
  脚本里点它等于点了别的行。要按「可见的那一个」过滤。
- 目录列表是按目录缓存的（`state.dirs`）：写完之后如果算的是「目标目录里有没有同名」，
  必须 `loadDir(..., force)` 现拉一次——缓存里那份可能还是空的，拿它判断就会覆盖别人的文件。
- `@trimjs/web-app` 的初始化 Promise，在页面被塞进一个**不是飞牛宿主**的 iframe 时
  **永远不 settle**：此时 `isStandaloneWeb` 是 false（它只看 `window.parent !== window`），
  但对面根本不回应握手。表现是所有宿主能力调用一直挂着——文件选择器点了没反应、界面
  按钮一直转圈，也不报错。所以 `trim/sdk.ts` 里调用前先 `ensureHostReady()` 等一个上限
  （10 秒），超时就报「宿主桥没有就绪」。
- `trim.file.convertPath` 的返回结构**和官方文档不一样**：文档写 `{status,result}`，
  实测系统直接回数组（`[{path,semanticPath}...]`）。`trimapi.decodeSemanticPaths` 三种
  形态都认，别照文档钉死结构，否则用户看到的是「响应数据格式不符合预期」+ 界面退回
  显示 `/vol1/...`。返回的 `path` 也可能与请求的写法不一致（多余的尾斜杠），
  `utils/semantic.ts` 专门做这层对上号、`check:semantic` 盯着它；`language` 是必填项
  且从**请求体**读（查询串会被反代丢掉）；`data.status` 非 0 同样是失败。
- 项目目录与输出目录**撞在一起**时有一套优先级：界面里用户明确指定的值严格校验
  （报错），环境变量撞了则保项目目录、输出退回默认；连默认值都撞（项目目录 =
  `$DATA_DIR/output`）时**不能拒绝启动**，按「没有单独的输出目录」处理
  （`Manager.outputIsProject` 兜住不重复复制、不重复列产物）。把部署配置判成错误
  只会让服务起不来，而实际后果只是多余的复制。
- 换授权目录时**先完成新目录的授权与切换（save 落盘），再删旧授权**。反过来的话，
  用户在选择器里取消、或者选择器直接报错，旧目录的授权已经删了而配置里还写着它——
  应用会突然读不到自己的项目根目录。
- 宿主在用户**直接关掉选择器**时返回 `code: 1` + `msg: "Operation failed"`（没有业务码、
  没有可读原因）。别把它当错误抛出去——用户只是点了取消。`trim/sdk.ts` 把 `code: 1` 和
  「成功但没返回路径」都归一成空数组，调用方给一句「已取消」。

## 交付前自检

1. `cd frontend && npm run typecheck && npm run check`；
2. 动了 Go：`make test`（必要时再 `go vet ./...`）；
3. 改了界面 / 交互：按「怎么真跑起来」跑一遍，点一遍改动的路径，别只看编译过没过；
4. 改了行为或界面约定：同步 README.md（它是这个项目唯一的设计文档）；
5. 报告里写清楚**改了什么、怎么验的、哪一条没验**，没验的别写成验过了。
