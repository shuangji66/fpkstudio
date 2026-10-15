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
npm run check            # 十一个检查脚本：i18n 键一致 + 向导往返 + 取景框几何/图标路径 + 命名与文本格式
                         #   （含 manifest 回写）+ 多选下拉的标签排版 + 首页分页排版
                         #   + 创建副本的文本改写 + 编辑器脏判据/切视图交棒 + 语义路径
                         #   + 文件管理器 store（重命名 / 保存 / 剪贴板）
                         #   + 日志 store（后端重启后的去重游标）
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
                             以及左栏文件树、文件标签；useCloseFile / useUpload / useDownload /
                             useOpenFolder 四个小组合函数也在这里（关标签的未保存提醒、
                             带进度的上传、下载并报错（目录挂「正在打包」提示）、
                             在文件管理器里打开项目目录）
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
- **`Manager` 的目录字段只能通过快照读**：`SetRoots`（设置弹窗里换目录）会用
  `atomic.Pointer` 换掉一整套（项目根 / 输出目录 / 各自的绝对路径 / `prjInHome`），读方走
  `m.snap()` / `m.prjRoot()` / `m.outputRoot()`，一次操作里只取一次快照（`Pack` 就是这么做的）。
  别去读裸字段：`go test -race` 会报数据竞争，而且真会出现「新的项目根 + 旧的项目根相对路径」
  这种混搭（打包照着旧位置复制内容）。
- **文件管理器有两个视图**（左栏文件树 / 「文件」标签页，见 `stores/workspace.ts` 的
  `viewMode`），它们是同一批文件的两个入口：目录列表缓存（`state.dirs`）和剪贴板
  （`state.clipboard`）都放在 store 里共用，**别在组件里各留一份**——各留一份的表现是
  「在标签页里复制、切回文件树粘贴不了」。剪贴板粘一次就清空（复制和剪切都是），别把它
  当成「能反复粘」的剪贴板。同理，「关掉一个文件标签」的未保存提醒在
  `components/project/useCloseFile.ts`，桌面标签和移动端子页面都用它。
- **`prepareStage` 不许预建目标目录**。它先 `Remove` 掉 `app/server` 再整份复制（这是
  「替换」语义，不是合并），而复制走的是 `Root.Copy` / `fsx.CopyTree`——两者对**已存在的
  目标**都返回 `ErrExists`。以前那里多了一句 `Mkdir`，于是默认部署（项目根在数据目录内，
  走 `m.home.Copy`）下多架构打包 100% 失败，界面只看到一句「目标已存在」；项目根在数据
  目录之外（走 `CopyTree`）时反而是好的，所以很容易漏掉。回归在
  `internal/projects/pack_test.go`。
- **日志环的裁剪判据踩过 `cap` 的坑**：`append` 之后 `len > cap(l.ring)` 永远不成立
  （要么没超、要么 append 刚把 cap 翻倍），那段代码曾经是死代码、环从不封顶。上限要单独
  存（`Logger.ringMax`），`ring_test.go` 钉住了。
- **剪切粘贴不要复用 `renameEntryAt`**：它是「同目录改名」，加了「目标必须在同一目录」的
  守卫；拿它当移动用会让同目录粘贴被判重名、跨目录直接报「名称不合法」——剪切功能整个
  不可用。`check:workspace` 里有跨目录剪切粘贴的断言。
- **「表单 / 源码」切换时该交哪份文本，规则在 `utils/handoff.ts`**（纯函数，`check:editor`
  钉住）：表单模型不可信（编辑器报了 `saveBlocked`）时交回**表单被加载时的那份文本**，
  不能交 `getContent()` 的回写结果——解析失败降级出来的是一份空配置，而 `[]` /
  `{"defaults":{"run-as":"package"}}` 这类空配置往往还「合法」，源码视图的 `checkFormat`
  拦不住，一保存就把用户的文件清空。
- **「保存」相关的三条路都要问 `saveBlocked`**：`save()`、`overwriteWizards()`（覆盖其他
  向导）、以及源码视图的 `checkFormat`。少问一条的表现是「换个按钮就把用户文件写坏」。
  `ManifestEditor` 的 `saveBlocked` 判的是 `Manifest.unterminated`（多行值 `"""` 没闭合
  ——解析器会一路读到文件尾、把后面的键吞进值里，再保存就把那些键埋掉了）。
- **新建文件不许覆盖已有文件**：后端 `/api/fs/create` 对已存在的**文件**返回 400
  （`WriteFile` 是 `O_TRUNC`，同名会把用户那份清成 0 字节），前端 `createEntryAt` 先
  `loadDir(parent, true)` 现拉一次再判重名。目录保持 `mkdir -p` 的幂等（补架构目录依赖它）。
- **日志的去重游标认「后端进程」**：`logging.seq` 是进程内计数，重启从 1 开始；SSE 第一帧
  是 `event: boot`，前端 `stores/logs.ts` 靠它把 `lastSeq` 归零。不认这一层的话，重启之后
  新日志会被整片当成旧的丢掉（弹窗从此不再更新）。补历史和实时帧是并发跑的，所以
  `loadHistory` 只按 seq **合并**、不整体覆盖；`LogDialog` 的刷新 watch 盯最后一条的 `seq`，
  不能盯 `visible.length`（到上限之后长度恒定，watch 永不触发）。
- **表单行用 `form-row` / `form-label`**：编辑器的表单行挂这两个类（小控件再加
  `form-row-inline`），表单根节点在窄屏下挂 `form-stack`（`:class="{ 'form-stack':
  layout.isMobile }"`），桌面那套「固定宽度右对齐标题 + 控件」由元素上的 Tailwind 类
  保留、窄屏的竖排由 `style.css` 覆盖。新写表单照这个来，别自己再排一套。
  **manifest / 入口配置 / 运行身份这几份表单是另一套**（`ManifestEditor` /
  `UiConfigEditor` / `PrivilegeEditor`）：桌面就是标题一行、控件一行，一行里放一组字段
  （manifest 的 `rows`，一项 = 一行；列数按组里几个字段算，`grid-cols-*` 必须写字面量
  ——Tailwind 扫不到拼出来的类名），窄屏下 `form-stack` 再把分组拆成一行一个。运行身份
  只有一列，两种布局完全一样，所以它不挂 `form-stack`。**拆开后的顺序就是 DOM 顺序**，
  桌面上的配对和它不一致时（`rows` 里的 `grid`: manifest 的开发者 / 发布者那组）得用
  `grid-flow-col` 之类把它折回去，别靠改 DOM 顺序解决——那样窄屏会读错序。
  `ResourceEditor`（资源声明）也是「标题一行、控件一行」，只是它按块手写、没走
  `form-row` / `form-stack`，写它的时候照抄同一套排布（字号 14px：标签 `text-sm`、
  控件 `size="default"`，全局配的 `size: 'small'` 是 12px）。
  **总览 / 快捷设置 / 图标这页三个面板也是 14px**（`OverviewPanel` / `ShortcutsPanel` /
  `IconPanel` / `IconTargetRow`）：文本 `text-sm`、带字的控件 `size="default"`，
  `el-tag` 要挂 `class="text-sm"`（它的字号写死在 `--el-tag-font-size` 里，size 只改
  高度和内边距），不带字的图标按钮保持 `size="small"`。
- **多选下拉（`multiple` + `collapse-tags`）一律用 `components/editor/TagSelect.vue`**，
  别直接写 `el-select`：`collapse-tags` 是按**个数**折叠的（`max-collapse-tags`，默认 1），
  根本不看容器多宽——600px 宽的字段也会显示成「第 1 个 +3」。TagSelect 每轮排版量一次
  标签的真实宽度，宽度够就全显示、放不下才折叠（算术在 `utils/tags.ts`，进 `check:tags`）。
  它的 `options` 收字符串或 `{ label, value }`，`class` / `placeholder` / `popper-class`
  照原样透传；**没挂 `collapse-tags` 的多选不算这类问题**（它会自己折行，不隐藏），
  不用改。
- **弹窗只有一种形态**：所有 `el-dialog` 都挂 `class="app-dialog"` +
  `modal-class="app-modal"`（样式在 `style.css`：居中、正文自己滚、遮罩模糊），
  再加 `align-center` / `lock-scroll` / `append-to-body`。**新加弹窗照抄这三个属性和
  两个类**，别再各写一套尺寸——「每个弹窗长得都不一样」正是这次统一掉的东西。
  高度默认固定 90% 视口高；内容不多的再挂 `app-dialog-auto`（高度跟内容走，上限仍是
  `min(90vh, var(--vv-height) - 2rem)`），别让几行的设置弹窗空出半个屏幕。
  弹窗标题是 Element Plus 的 18px 默认值，**正文一律 14px**（log 那条 xterm 也是）。
- **语言包两份必须同步**。`npm run check:i18n` 会挡住键不一致；消息里的 `{` 是插值语法，
  想写字面量 `{` 要转义。一条消息写错，表现是「渲染到那个组件时整个弹窗画不出来」。
- **别把「项目的 i18n 目录」和「本应用的语言包」搞混**。`src/i18n/zh-CN.ts` 是这个工具
  自己的界面语言；项目里的 `i18n/en-US` 是**被打包的 fpk** 的文案，官方语法是 **INI**
  （`[section]` + `key="值"`），**不是 JSON**——`utils/validate.ts` 按 INI 校验（判成 JSON
  的后果是「一份合法的 locale 文件在源码视图里存不进去」）。官方还要求「有 `i18n/`
  目录就必须提供 `en-US`」：`Pack` 会拒（后端 `checkI18nLocale`），界面在快捷设置的
  i18n 分组提示、打包前也拦一道。**前端的判据走目录缓存**（`workspace.missingEnUS`，
  i18n 目录没拉过 = 「还不知道」= 不报），所以需要准确的地方（拦打包）要先
  `loadDir(..., true)` 现拉一次；目录存在与否、缺哪个语言别去读裸字段或自己拼字符串，
  用 `workspace.i18nDirOf`。
- **`utils/` 里不引 `vue-i18n`**：需要给用户看的错误在 `utils/` 里只给 **i18n 键**
  （见 `utils/icon.ts` 的 `IconError`，界面层用 `IconPanel` 的 `errText` 翻）——否则
  `check:icon` 这类直接跑 `.ts` 的脚本会被拖上一整套运行时。
- **改目录设置要过权限**（`server/api_trim.go` 的 `checkDirsAuthorized`）：经过统一网关时
  要求管理员，且新目录必须在开放 API 报的「本应用被授权目录」里；没有网关（直接调 socket /
  开发）时不判——那种部署下 socket 权限就是边界。
- **`utils/` 里的纯逻辑要能单独验**。解析 / 回写 / 几何这类会写坏用户文件或算错图的计算，
  放 `utils/` 并在 `frontend/scripts/check-*.mjs` 里加断言，别塞进组件里只靠手点。
- **首页项目的两种视图**（`HomeTab.vue`）：平铺 = `auto-fill` 自适应列宽 + 整页滚动；
  分页 = 一页铺满容器 + 顶栏翻页。每页几个由 `utils/paging.ts` 的 `fitGrid` 从项目区的
  实际尺寸算（`ResizeObserver` + 扣掉 `p-4` 内边距），**别写死一个每页数**：窗口一变就
  会露出「底部空一条」或者「最后一排被切掉」。分页网格的行高必须 `1fr` 均分，列数 / 行数
  走 inline style（拼出来的 `grid-cols-${n}` Tailwind 扫不到）。列表视图已经删掉了，
  别再往 `ProjectCard` 里加第二种排布。**换页栏在顶栏**（工具条下面）、**项目计数 + 称号
  在最后一行卡片下面**（居中 + 分隔线，两种视图都有）；称号分六档（`rankKeyFor`，边界归
  下档），分页算每页几个时要把计数行的高度扣掉（`footReserve`）。
- **前端只用 Composition API + `<script setup lang="ts">`**，SFC 段落顺序
  `<script>` → `<template>` → `<style>`。组件超过一个明确职责就拆。
- **UI 文案全部走 i18n**，不要在组件里硬编码中文（历史遗留：manifest 的
  `platform` 选项直接写在字段表里，改文案时注意两份语言包都要动）。
- **界面字号统一 14px**（现在首页和弹窗也是）：带字的控件显式 `size="default"`
  （全局配的是 `size: 'small'` = 12px），`el-tag` 还要 `class="text-sm"`（它的字号
  写死在 `--el-tag-font-size` 里，size 只改高度内边距）；**只有图标**的按钮保持
  `size="small"`。子标签字号在 `style.css` 的 `.subtab`，`.label-text` 那类 12px 的
  标签在新写的界面里直接用 `text-sm font-medium ...`。唯一的大字号是设置弹窗里的
  「关于」标题（`text-lg`，和弹窗自己的标题「设置」一样大——那本来就是
  `--el-dialog-title-font-size` 的默认值）。
- **「创建副本」只改两处文本**（`utils/duplicate.ts` + `check:duplicate`）：`manifest` 的
  `appname` 与 `desktop_applaunchname` 的前缀、`app/ui/config` 里 `.url` 每个 id 的前缀
  （入口 id 是 `<appname>.<入口名>`，只换第一个点之前那一段；没有点的 id 原样留着）。
  其余文件**原样复制**，别顺手改。目标重名由后端挡住（`Root.Copy` 对已存在的目标直接回
  `ErrExists`，绝不覆盖），前端那次 `projectExists` 只是把提示写得更直白。config 解析不了时**不写回**
  （和「解析失败就拦保存」同一条理由），只报一句。
- **删除按钮红色、重要的删除要确认**：`text` + `type="danger"`；删掉一整块内容
  （步骤、入口、共享目录、图标组、项目、文件）之前 `ElMessageBox.confirm`，只丢一行的
  （字段 / 选项 / 校验）不问——逐条问会让人不敢动手。
- **左栏文件树的宽度上限是窗口宽度的一半**（`ProjectTab.vue` 的 `treeMax()`，每次拖
  现算 + 从 localStorage 读回来时钳一次）：别写死像素值，窄窗口上会把编辑区挤没。
  拖动有两个入口：那条 1px 的分隔线，和树顶栏右侧吸在分隔线上的手柄按钮（`FileTree`
  抛 `resize-start` 事件，逻辑仍在 `ProjectTab`）。手柄要带 `touch-none`，否则触屏会把
  `pointermove` 当成滚动手势收走；手指点不中 1px 的线，所以这个按钮是**必须**的。
- **图片预览（`ImagePreview`，含 svg 的预览面）没有可存的内容**：`EditorHost` 按
  `registry` 里的 `previews` + 当前视图把「还原 / 保存」藏掉（`previewOnly`），
  别再让它们灰在那里占位置。
- **语言偏好（中文 / English / 跟随系统）在 `stores/locale.ts` + `i18n/index.ts`**，
  记在 localStorage 的 `fpkstudio.locale`（默认跟随系统）。切语言要**三处一起换**：
  vue-i18n 的 locale、`html lang`、Element Plus 的语言包——都收在 `applyLocale()` 里。
  Element Plus 的全局配置对象（`elementOptions`）**必须是 reactive 的**再交给 `app.use`，
  否则命令式 API（`ElMessage` / `ElMessageBox`）拿不到新语言。
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

- **关掉项目标签要连内存状态一起丢**（`workspace.forget`，挂在 `ProjectTab` 的卸载钩子
  上）。项目在磁盘上，应用外面随时可能被换掉（手动换文件、从别处导入一份、删掉再建一个
  同名的），状态留着就是「关掉再打开还是老内容，只有手动刷新才更新」，同名重建时还会把
  上一个项目的文件列出来。别改成在某个关闭入口里清——漏一个入口就少一条路生效；也正
  因为关标签会丢状态，「关闭其他标签」必须先问一声未保存的改动（`TabBar.closeOthers`）。
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
- **验「静态资源有没有缓存」别开着 DevTools 的「禁用缓存」**（Chrome 一开 DevTools 就默认勾着），
  也别用 Ctrl+Shift+R / Shift+F5——这两种都绕过缓存，看到的是「每个 asset 都 200 + 完整响应体」，
  和「响应头没生效」长得一模一样。三档要分清：头是 `immutable` 时**普通刷新一个 asset 请求都没有**；
  头是 `no-cache` 时是 **304**；全 200 才说明那次刷新绕过了缓存。别靠肉眼盯 DevTools，
  用 `curl -D-` 看头、用 `performance.getEntriesByType('resource')` 的 `transferSize`（0 = 命中缓存）
  或用反代/网关自己的 access log（它记的是真的请求）来判。
- 系统 `tar` 对越界成员（`/etc/passwd`、`../x`）**只给警告、退出码还是 0**，靠它挡不住；
  `unzip` 更省事——它把越界名字截断成一个安全名字照写不误，退出码给 1。所以
  `fsx.Extract` 解压前先列一遍成员名（tar 用 `tar -t`、zip 用 `unzip -Z1`）自己判，
  要动这段的话别把成员校验去掉。归档里的**符号链接一律不还原**（`fsx.sanitizeExtracted`，
  tar / zip 都一样）：链接可以指向归档外面，留在项目里会被后面的复制 / 打包顺着读出去。
- **重命名不等于「改个名字」**，`stores/workspace.ts` 的 `renameEntryAt` 里三件事要一起做对：
  打开着的标签要跟着改路径（编辑器宿主是按 `file.path` 做 key 的，key 一变组件就重建，
  内容只能从 `draft || origin` 取——顺手清掉 `origin` 的表现是「改个文件名，编辑框空了，
  随手敲一个字保存，文件就只剩那一个字」）；目录改名要连**子标签**一起改（否则保存照着
  旧路径写回去，凭空复活一个文件）；目标重名要拒绝（`rename(2)` 会直接覆盖同名文件，
  后端 `fsx.Rename` 与 store 各挡一道）。这三条钉在 `check:workspace` 里。
- `EditorHost.onBeforeUnmount` 交草稿时用的是**标签对象的引用**（`tabRef`），不是
  `props.path`：重命名会让两者不一样，用旧路径查不到标签，那份没保存的内容就丢了。
- 切换桌面 / 移动布局会把编辑器宿主整块卸载重建（`ProjectTab` 里用 `v-if="!mobile"` 换了
  另一套宿主）。编辑器里没保存的内容靠 `FileTab.draft` 在 `EditorHost.onBeforeUnmount`
  里接力；**新增编辑器组件时，`getContent()` 要能拿到当前文本**，否则这份接力会断，
  表现是「切一下布局，改到一半的内容没了，但脏标记还亮着」。
- **脏标记的两套判据要对齐**：表单编辑器说「脏」是「模型被改过」（`useEditor` 的基线 =
  磁盘那份走一遍解析 + 回写，**不是磁盘原文**），源码视图说「脏」是「文本 ≠ 磁盘原文」。
  把基线写回 `props.original` 原文、或者切视图时把表单的回写结果当源码视图的初值，
  表现都是「一份没编辑过的脚手架文件（fnpack 的 `config/privilege` 是 4 空格缩进、
  结尾不带换行）一按『源码』就显示未保存、保存按钮跟着亮」。切视图那条在
  `EditorHost.vue` 的 `watch(mode)`：表单干净就交 `tab.origin`。`check:editor` 盯着
  前一半，后一半只能按「怎么真跑起来」手点。
- **目录改名不止改标签路径**：`renameEntryAt` 里还要一起改**活动标签**（`activeTab` 是面板
  可见性的唯一判据，少了它编辑区整片空白）、目录缓存与展开状态（`migrateDirKeys`：不搬就
  「改完名变回收起」+ 旧键那份缓存变成孤儿）、以及剪贴板里的路径（不换的话之后每次粘贴都拿
  一个不存在的路径去改名 / 复制）。删除走 `dropDirKeys` + 清剪贴板。`check:workspace` 全钉住。
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
- 触屏上没有「把鼠标移开」这个动作：工具栏按钮的自绘提示（`el-tooltip`）点一下就
  **一直挂在屏幕上**挡着内容；自绘的那一层还得自己管显隐，鼠标点完按钮移开时也容易
  留在屏幕上不消失。**工具栏按钮的说明一律挂原生 `title`**（同时给 `aria-label`）：
  系统提示由浏览器接管，移开即消失、触屏上根本不出现。文件管理器的两个工具栏
  （`FileTree.vue` / `FileBrowser.vue`）已经全部改完；只有要富文本说明的地方
  （`ManifestEditor.vue` 字段旁、`ResourceEditor.vue` API 授权项旁那个问号）还留着
  `el-tooltip`。**新加工具栏按钮不要再加 `el-tooltip`**。
- **关闭按钮只有一种形态：红色小叉，且「悬停才出现」要认设备能力**。标签上的关闭按钮
  挂 `class="close-x hover-reveal"`（样式在 `style.css`），弹窗的（`el-dialog` /
  `el-message-box`）也在 `style.css` 里统一覆盖成红色——**不要**再各自写颜色类。
  **不要**用 `opacity-0 group-hover:opacity-100` 写「悬停才出现」：Tailwind 4 给 hover /
  group-hover 变体自带 `@media (hover: hover)`，没有指针悬停的设备（触屏平板）上那条
  规则永远不生效，元素就一直停在 `opacity-0`——按钮等于不存在（平板上就是这么丢的）。
  `.hover-reveal` 反过来写：默认常显，只有能悬停的设备才收起来（父元素要挂 `group`）。
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
  一个命令覆盖全部 tar 变体。**zip 也走系统命令 `unzip`**（`fsx.UnzipBin`），不再用
  标准库 `archive/zip`：解压统一交给系统工具，行为跟用户在终端里敲一遍一致，边角情况
  （属性位、跨平台名字、加密包）不用我们自己维护。`unzip` 必须带 `-o`——不给的话它遇到
  同名文件会停下来等 stdin，而服务进程没有终端，请求会一直挂着。
- **归档后缀有两份判据，改一处必须改两处**：后端 `fsx.archiveKinds`（长的排前面，
  `.tar.gz` 必须在 `.gz` 之前）和前端 `utils/format.ts` 的 `isArchive`。少一边的表现是
  「菜单里没有解压」或者「点了报不支持的格式」；`check:format` 盯着前端那一半。
- **解压结果里只留普通文件与目录**。交给外部 `tar` / `unzip` 的是**绝对路径**，`os.Root`
  在那一步帮不上忙，所以：归档本身必须是沙箱里的普通文件（`fsx.requireRegularFile`，
  指向外部的符号链接要拒绝）；解完之后 `fsx.sanitizeExtracted` 把符号链接与非普通文件
  （FIFO、设备节点）都删掉再搬进项目——FIFO 没有写者时 `open(2)` 永久阻塞，留着它等于
  留一个「点一下就挂死」的条目。删掉的数量要报给界面（`links` / `specials`）。
- **外部命令都要带 ctx 与上限**。`Extract(ctx, rel)` 走 `exec.CommandContext`（客户端断开
  就杀掉子进程），服务端再套一个 10 分钟上限；`ReadFile` / `OpenFile` 拒绝非普通文件
  （同一个阻塞理由）。
- **读、上传、导入都有体积上限**：`/api/fs/read` 超过 `maxReadBytes`（8MB）只回元信息 +
  `tooLarge:true`；上传 / 导入单个请求 `maxUploadBytes`（1G，`http.MaxBytesReader`）。
  这两个是**变量**，测试里调小用（`maxImportFileBytes` 同理，它管的是「走飞牛选择器
  导入的那个包本身」，那条路没有请求体可卡）。上传 / 导入要支持取消：写入用 `ctxReader`
  包一层，ctx 一取消就退出，半截文件删掉。
- **HTTP 状态码按语义分**：目标不存在 404（前端按「还没创建」处理）、路径不合法 /
  目标是目录 / 目标已存在 400 与 409、没权限 403、超限 413、方法不对 405，其余 500。
  `fsx` 的 `ErrInvalidPath` / `ErrExists` / `ErrNotExist` 就是给这层判的；handler 里用
  `fsError(...)` 或 `projectError(...)`，别再一律 `internal(...)`。
- **产物文件名只由受校验的成分拼成**。`<appname>-<version>-<platform>.fpk` 的三个成分都
  来自 manifest，拼之前过 `safeArtifactPart`（不含分隔符 / 控制字符、不是 `.` / `..`、
  ≤64 字节），落地走 `Root.WriteReader`（`copyIntoRoot`）——绝对路径 + `filepath.Join`
  会把 `..` 规整掉，等于把产物写到项目根之外。`serverDir` 也不许填 `.`。
- **移动与重命名是两条路**：`renameEntryAt` 只负责「同目录改个名字」（目标必须在同一目录，
  否则用户按「重命名」会被搬走）；跨目录搬文件走 `moveEntryAt`（`pasteInto` 的剪切分支）。
  两者共用 `applyPathChange` 做内存里的迁移，别各写一份。
- **日志环按 `ringMax` 裁，不要拿 `cap(l.ring)` 当上限**：`append` 会先把容量翻倍，
  `len > cap` 在 append 之后永远不成立。
- 编辑器的「解析失败」如果只是降级成空模型（`utils/wizard.ts` 的 `parseWizard` 就是
  这样：解析不了返回 `{steps: [], error}`），那 `getContent()` 交出去的就是一份空配置，
  点保存等于把用户的文件覆盖掉。这种编辑器必须实现 `EditorApi.saveBlocked()`，
  EditorHost 会在取内容之前先问它。**源码视图是另一条路**，它不走表单组件，校验在
  `utils/validate.ts` 的 `checkFormat`（SourceEditor 的 `saveBlocked` 调它）——
  改格式判据要两边都看：`checkFormat` 决定源码视图拦不拦，`saveBlocked` 决定表单视图。
  判据放在 `utils/` 是为了能进 `check:format` 单独验。
- **`fsx.ZipDir` 默认带一层和目录同名的最外层目录**（只有 `path` 为 `.` 时省掉，那时没有
  目录名可取）：项目导出和文件管理器的「下载」都走它，摊平会把 `manifest` / `app/` 直接撒
  在用户选的目标目录里。**空目录也要留下这一个目录条目**——少它就得到一个没有任何成员的
  「空包」，解压工具直接报错。回归在 `fsx.ZipDirKeepsTopLevelDir`。
- **打包会清掉 manifest 的格式空格**（`stripManifestSpaces`，`Pack` 里对着**暂存副本**跑
  一次）：`appname   =   demo  ` → `appname=demo`。值内部的空格、`"""` 多行值的正文、
  空行与注释都不许动，只去 `key` / `value` 两侧和行尾的空白。判据必须和 `parseManifest`
  对齐——**先把值 TrimSpace 再判 `"""`**：不 Trim 的话带空格的对齐写法（官方模板就是
  `desc = """`）认不出来，多行值的正文会被当成普通行逐行去空格改坏（第一版就这么错了，
  `TestStripManifestSpacesKeepsValues` 钉住）。用户项目里那份 manifest 保持原样。
- **首页卡片置顶只存浏览器**（`utils/pins.ts` + `stores/projects.ts` 的 `pinned`，键是
  localStorage 的 `fpkstudio.pinnedProjects`，不进后端、不进 `state.json`）。顺序语义是
  「先置顶的排前面」，排序必须**稳定**（没置顶的保持后端顺序，用 `sortPinned`，别自己写
  filter + concat 打乱顺序）。项目**改名 / 删除时置顶表要跟着改**，否则会留下一条指向
  不存在项目的记录。断言在 `check:paging`。**卡片上那个置顶标记是自己画的 svg 图钉**：
  Element Plus 的图标集里没有图钉，用 `Top` 会是个向上箭头，摆在项目名旁边像「排序 /
  回到顶部」（`ProjectCard.vue` 里三个图元：钉帽、带法兰的钉身、针）。
- **「在文件管理器里打开项目目录」三处共用一个组合函数**（`components/project/useOpenFolder.ts`：
  首页卡片操作区、左栏文件树顶栏、「文件」标签页顶栏）。它按**项目名**去 `stores/projects`
  里查 `absPath`，别用 `projectsDir + name` 拼——项目根可以被换成授权目录。独立浏览器页面
  里置灰并说明原因（宿主能力），说明挂原生 `title`（工具栏按钮不许用 `el-tooltip`）。
- **manifest 的「默认入口」候选来自 `app/<入口目录>/config` 的 `.url` 键**，读法在
  `utils/entries.ts`（入口 id 只存在键名里）。这个字段要在**每次点开下拉时现读**一次磁盘上
  的 config（`ManifestEditor.refreshLaunchOptions`，并进当前值），不然「刚在入口配置里加的
  入口」在下拉里选不到；读不到不弹错、照样能手输。断言在 `check:duplicate`。
- **「插入模板」按路径给，`cmd/main` 还按项目类型分两份**（`EditorHost` 的 `templateFor` +
  `utils/lifecycle.ts`）：非 docker 的启停脚本自己托管进程（pid 文件 / TERM / KILL），
  docker 应用交给应用中心、脚本只做 status；两份都是官方脚手架的原样文本，别「优化」写法。
  项目类型取 `workspace.state(project).docker`（`ensure()` 填的，项目标签打开时一定就位），
  不要去读项目列表。模板是模板字面量，**`${...}` 少转义一个反斜杠会被当插值吃掉**（脚本里
  的变量就没了，而且大概率不报错），所以 `check:editor` 钉住两份模板的归属、变量写法与
  关键语句。新增模板照这个来：文本放 `utils/lifecycle.ts`（能单独验），组件里只留
  「哪个路径用哪份」的判断。

## 交付前自检

1. `cd frontend && npm run typecheck && npm run check`；
2. 动了 Go：`make test`（必要时再 `go vet ./...`）；
3. 改了界面 / 交互：按「怎么真跑起来」跑一遍，点一遍改动的路径，别只看编译过没过；
4. 改了行为或界面约定：同步 README.md（它是这个项目唯一的设计文档）；
5. 报告里写清楚**改了什么、怎么验的、哪一条没验**，没验的别写成验过了。
