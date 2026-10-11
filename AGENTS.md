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
  stores/                    pinia：projects / tabs / workspace / system / logs / theme / layout
  components/project/        项目内的固定页：文件（标签页模式）/ 总览 / 快捷设置 / 图标，
                             以及左栏文件树、文件标签；useCloseFile / useUpload 两个
                             小组合函数也在这里（关标签的未保存提醒、带进度的上传）
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
- **文件管理器有两个视图**（左栏文件树 / 「文件」标签页，见 `stores/workspace.ts` 的
  `viewMode`），它们是同一批文件的两个入口：目录列表缓存（`state.dirs`）和剪贴板
  （`state.clipboard`）都放在 store 里共用，**别在组件里各留一份**——各留一份的表现是
  「在标签页里复制、切回文件树粘贴不了」。剪贴板粘一次就清空（复制和剪切都是），别把它
  当成「能反复粘」的剪贴板。同理，「关掉一个文件标签」的未保存提醒在
  `components/project/useCloseFile.ts`，桌面标签和移动端子页面都用它。
- **表单行用 `form-row` / `form-label`**：编辑器的表单行挂这两个类（小控件再加
  `form-row-inline`），表单根节点在窄屏下挂 `form-stack`（`:class="{ 'form-stack':
  layout.isMobile }"`），桌面那套「固定宽度右对齐标题 + 控件」由元素上的 Tailwind 类
  保留、窄屏的竖排由 `style.css` 覆盖。新写表单照这个来，别自己再排一套。
- **弹窗只有一种形态**：所有 `el-dialog` 都挂 `class="app-dialog"` +
  `modal-class="app-modal"`（样式在 `style.css`：90vh、居中、正文自己滚、遮罩模糊），
  再加 `align-center` / `lock-scroll` / `append-to-body`。**新加弹窗照抄这三个属性和
  两个类**，别再各写一套尺寸——「每个弹窗长得都不一样」正是这次统一掉的东西。
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
- **「目标不存在」不记日志**（`Server.logFsMiss`）：向导 / 生命周期脚本常常还没创建，
  图标目录也可能没有，这些都是正常状态，会以 404 回给前端、前端按「未创建」处理。
  真错误（权限、EISDIR…）照记。启动日志同理只留一行「已启动」，运行参数要看就
  `./fpkstudio -paths`。
- `/api/fs/*` 的 `path` 是相对**项目根目录**（`projects/` 这个沙箱根）的，也就是
  `demo/app/server` 这种带项目名的写法；返回里的路径（解压结果的 `dir`）同样是这一套。
  **别再加一次项目名前缀**：算出来的缓存键（`state.dirs` 的键）根本不存在，`refreshDir`
  会静默什么都不做，表现是「解压成功了，但列表里看不到新文件」。
- 系统 `tar` 对越界成员（`/etc/passwd`、`../x`）**只给警告、退出码还是 0**，靠它挡不住；
  `fsx.Extract` 是自己先用 `tar -t` 列成员名再判的。要动这段的话，别把成员校验去掉。
- 切换桌面 / 移动布局会把编辑器宿主整块卸载重建（`ProjectTab` 里用 `v-if="!mobile"` 换了
  另一套宿主）。编辑器里没保存的内容靠 `FileTab.draft` 在 `EditorHost.onBeforeUnmount`
  里接力；**新增编辑器组件时，`getContent()` 要能拿到当前文本**，否则这份接力会断，
  表现是「切一下布局，改到一半的内容没了，但脏标记还亮着」。
- **`style.css` 的层序会吃掉你的规则**：`@layer theme, base, element, components, utilities`
  是级联层，**层序比选择器权重更硬**——写在 `components` 层里的规则永远压不过元素上
  直接写的 Tailwind 工具类（`.grid` / `.text-right` 都在 `utilities` 层），表现是
  「规则写了、完全没生效」。要覆盖元素上的工具类，规则得放进 `@layer utilities`，
  或者干脆别在元素上写那个工具类。`.form-stack` 那组就踩过这个坑。
- **虚拟键盘不会让布局视口变矮**：iOS 只缩可视视口，Android 默认也只缩可视视口
  （要连布局一起缩得靠 viewport meta 的 `interactive-widget=resizes-content`，只有
  Chromium 认）。整块高度写死 `100%` 的应用（就是这个）底部会被键盘盖住。
  适配在 `utils/viewport.ts`：把**可视视口的高度和上沿偏移**写进 `--vv-height` /
  `--vv-top`，`style.css` 里 `#app`（`position: fixed` + `top: var(--vv-top)`）和
  `.el-overlay` 都按它定位，`.app-dialog` 用 `min(90vh, var(--vv-height) - 2rem)` 让位。
  为什么不是「减掉键盘高度」：**iOS 允许把可视视口拖着平移**，只缩高度的话下面会露出空白，
  表现是「拖到底还能继续拖，页面像变大了」——钉在可视视口上才是真的贴住键盘。
  公式对 iOS、认 meta 的 Android（变量和 `innerHeight` 一致，不重复缩）、桌面（等于整个
  视口）都成立，所以**不要**给它加 `layout.isMobile` 判断。
  另外 `html` / `body` 设了 `overflow: hidden` + `overscroll-behavior: none`，`.thin-scroll`
  设了 `overscroll-behavior: contain`：滚到头不把滚动链交给外层，整页就不会被拖着弹。
- 改 `.el-overlay` 的尺寸要**连 `height` 一起写**：Element Plus 自己给了
  `top:0; bottom:0; height:100%`，三者同时存在时 CSS 会忽略 `bottom`，只改 `bottom`
  等于没改（适配键盘时踩过）。
- `#app` 是 `position: fixed` 的（钉可视视口），**别再给它加 `transform`**：transform 会
  给里面的 `position: fixed` 元素换一个包含块，弹层会全歪。弹窗是 teleport 到 body 的，
  不受影响。
- 触屏上没有「把鼠标移开」这个动作：工具栏按钮的 `el-tooltip` 点一下就**一直挂在屏幕上**
  挡着内容。文件管理器工具栏的提示都给了 `:hide-after`（`TOOLTIP_HIDE`），**移动端干脆
  整个禁用**（`:disabled="layout.isMobile"`）；新加提示按钮记得跟上。同理
  `opacity-0 group-hover:opacity-100` 这种「悬停才出现」的按钮在触屏上等于不存在
  （项目标签的关闭按钮就踩过），移动端要常显。
- **移动端输入框字号是 16px**（`style.css`）：iOS Safari 聚焦小于 16px 的输入框会把整个
  页面放大且不缩回去，`user-scalable=no` 从 iOS 10 起被忽略，只能靠字号挡。别把窄屏下
  的输入框字号改回 12–14px。
- **跟「移动布局」的样式要跟布局开关，别用宽度媒体查询**：布局可以在设置里手动钉死，
  而媒体查询只看真实视口宽度，表现是「明明在移动布局里，样式还是桌面的」——CodeMirror
  的 16px 就这么写过一次。要么用 `layout.isMobile` 挂个类（`.cm-mobile`），要么把规则
  放进 `@media (max-width: 767px)` 时想清楚它到底该跟谁。**例外**：iOS 输入框放大那条
  必须用宽度媒体查询，因为它取决于真实视口。
- 解压工具默认按 `/usr/bin/...` 找（`fsx.binPath`），找不到才回落到 PATH：服务进程的
  PATH 不保证有 /usr/bin，而这些工具在飞牛上就在那儿。不给它们加环境变量。
  tar 的命令**不带 `-z` / `-j` / `-J` / `--zstd`**：GNU tar 读归档时按魔数自己认压缩格式，
  一个命令覆盖全部 tar 变体。**zip 用标准库 `archive/zip`**（`unzip` 不是每个系统都有）。
- **归档后缀有两份判据，改一处必须改两处**：后端 `fsx.archiveKinds`（长的排前面，
  `.tar.gz` 必须在 `.gz` 之前）和前端 `utils/format.ts` 的 `isArchive`。少一边的表现是
  「菜单里没有解压」或者「点了报不支持的格式」；`check:format` 盯着前端那一半。
- 编辑器的「解析失败」如果只是降级成空模型（`utils/wizard.ts` 的 `parseWizard` 就是
  这样：解析不了返回 `{steps: [], error}`），那 `getContent()` 交出去的就是一份空配置，
  点保存等于把用户的文件覆盖掉。这种编辑器必须实现 `EditorApi.saveBlocked()`，
  EditorHost 会在取内容之前先问它。**源码视图是另一条路**，它不走表单组件，校验在
  `utils/validate.ts` 的 `checkFormat`（SourceEditor 的 `saveBlocked` 调它）——
  改格式判据要两边都看：`checkFormat` 决定源码视图拦不拦，`saveBlocked` 决定表单视图。
  判据放在 `utils/` 是为了能进 `check:format` 单独验。

## 交付前自检

1. `cd frontend && npm run typecheck && npm run check`；
2. 动了 Go：`make test`（必要时再 `go vet ./...`）；
3. 改了界面 / 交互：按「怎么真跑起来」跑一遍，点一遍改动的路径，别只看编译过没过；
4. 改了行为或界面约定：同步 README.md（它是这个项目唯一的设计文档）；
5. 报告里写清楚**改了什么、怎么验的、哪一条没验**，没验的别写成验过了。
