import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  copyEntry,
  createEntry,
  deleteEntry,
  downloadEntry,
  extractEntry,
  getArtifacts,
  getPlatforms,
  getSettings,
  listDir,
  packProject,
  projectExists,
  readFile,
  renameEntry,
  saveSettings,
  uploadFiles,
  writeFile,
} from '@/api/endpoints'
import { ApiError } from '@/api/client'
import type { Artifact, ExtractResult, FsEntry, PackResult, PackSettings } from '@/api/types'
import { i18n } from '@/i18n'
import { uniqueName } from '@/utils/format'
import { useLogsStore } from './logs'

/** 项目标签内固定的「总览」子标签 id（原来的「打包」页）。 */
export const OVERVIEW_TAB = '__overview__'
/** 固定子标签：表单快捷跳转。 */
export const SHORTCUTS_TAB = '__shortcuts__'
/** 固定子标签：图标统一处理。 */
export const ICON_TAB = '__icon__'
/**
 * 固定子标签：文件管理器。
 *
 * 只有标签页模式（`viewMode === 'tabs'`）才有这个标签——左栏文件树模式下文件树
 * 就长在侧边栏里，再给一个同名标签是重复的。
 */
export const FILES_TAB = '__files__'

/**
 * 进入项目时默认停在哪。
 *
 * 「快捷设置」是打开项目之后最常去的一页（改 manifest / 权限 / 脚本），总览更多是
 * 打包时才看，所以默认落在快捷设置上。
 */
export const DEFAULT_TAB = SHORTCUTS_TAB

/** 文件管理器的两种形态。 */
export type ViewMode = 'tree' | 'tabs'

/** 标签页模式里文件列表的两种排布。 */
export type BrowserView = 'list' | 'grid'

/**
 * 粘贴的结果。
 *
 * 只回状态、不回文案：给用户看的那句话要按当前语言渲染，那是组件的事。store 里
 * 只用 i18n 取「副本」这个后缀——那是个取值，不是一句话。
 */
export type PasteOutcome = 'ok' | 'exists' | 'unreadable' | 'invalid'

/** 固定子标签的 id 一律是 `__xxx__` 这种形状；文件子标签用的是文件路径。 */
const isFixedTab = (id: string) => id.startsWith('__') && id.endsWith('__')

const viewModeKey = (name: string) => `fpkstudio.viewMode.${name}`

function readViewMode(name: string): ViewMode {
  return window.localStorage.getItem(viewModeKey(name)) === 'tabs' ? 'tabs' : 'tree'
}

export interface FileTab {
  /** 相对于项目目录的路径，例如 `config/privilege`。 */
  path: string
  /** 展示用文件名。 */
  name: string
  dirty: boolean
  loading: boolean
  error: string
  /** 是否为文本文件；二进制不给编辑器。 */
  text: boolean
  sizeRaw: string
  /** 打开时的原始内容，用于判断改动与「还原」。 */
  origin: string
  /**
   * 编辑器手里正在编、还没落到 `origin` 的内容。
   *
   * 只在编辑器被卸载时写进来（切换桌面 / 移动布局会重建编辑器宿主），让重新挂载的
   * 编辑器接着编。保存 / 还原后清空——它是「临时接力棒」，不是第二份真相。
   */
  draft: string
  /**
   * 磁盘上还不存在这个文件（读取返回 404）。
   *
   * 快捷设置里的向导 / 生命周期脚本经常是空的。这时按空内容打开、先在内存里编，
   * 保存时才真正落盘（`saveFile` 会把父目录一起建出来）。
   */
  missing: boolean
}

export interface ProjectState {
  name: string
  loaded: boolean
  loading: boolean
  error: string

  /** 目录路径 -> 该目录下的条目。懒加载，展开时才请求。 */
  dirs: Record<string, FsEntry[]>
  expanded: Record<string, boolean>

  files: FileTab[]
  activeTab: string
  /**
   * 打开文件之前停在哪（固定标签）。
   *
   * 关掉文件标签时退回这里，而不是无脑退回总览：移动端的「后退」就是靠它回到打开
   * 编辑前的位置（从「文件」标签点进去的，就该退回「文件」标签）。
   */
  fileReturnTab: string

  /** 文件管理器用哪种形态：左栏文件树 / 固定「文件」标签页。记在 localStorage。 */
  viewMode: ViewMode
  /**
   * 标签页模式当前所在的目录（含项目名的完整路径，和 `dirs` 的键同一套写法）。
   *
   * 和 `expanded` 不同，这里只是一个游标：进入子目录时压栈，后退 / 前进在栈之间
   * 来回搬，这样「后退」才真的是浏览器那种后退，而不是回到项目根。
   */
  browserDir: string
  browserBack: string[]
  browserForward: string[]
  /** 标签页模式的列表 / 网格排布（不进 localStorage，跟着会话走）。 */
  browserView: BrowserView
  /**
   * 复制 / 剪切剪贴板：只记路径，不搬内容。
   *
   * 放在 store 而不是组件里，是因为文件树和「文件」标签页是同一批文件的两个视图：
   * 在标签页里复制、切回文件树粘贴必须能连上。
   */
  clipboard: { path: string; cut: boolean } | null

  /** 是否 docker 应用 / 是否带桌面入口，决定快捷设置与图标页的可选项。 */
  docker: boolean
  ui: boolean

  /** 项目内文件被写入（保存图标等）后自增，让图片预览重新取图而不是吃缓存。 */
  assetRev: number

  platforms: string[]
  settings: PackSettings
  artifacts: Artifact[]
  packing: boolean
  results: PackResult[]
}

function emptyState(name: string): ProjectState {
  return {
    name,
    loaded: false,
    loading: false,
    error: '',
    dirs: {},
    expanded: {},
    files: [],
    activeTab: DEFAULT_TAB,
    fileReturnTab: DEFAULT_TAB,
    viewMode: readViewMode(name),
    browserDir: name,
    browserBack: [],
    browserForward: [],
    browserView: 'list',
    clipboard: null,
    docker: false,
    ui: false,
    assetRev: 0,
    platforms: [],
    settings: {
      multiPlatform: false,
      platforms: [],
      serverDir: 'app/server',
      copyToOutput: true,
    },
    artifacts: [],
    packing: false,
    results: [],
  }
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** 会被 <img> 直接渲染的路径：写完这些文件要让图片预览重新取图，别吃缓存。 */
const IMAGE_PATH_RE = /\.(png|jpe?g|gif|webp|bmp|avif|ico|svg)$/i

/**
 * 每个项目标签的编辑区状态：文件树、打开的文件、打包配置与产物。
 *
 * 打开的文件全部保持挂载（用 v-show 切换），所以未保存的改动不会因为
 * 切换标签而丢失——这也是「不用弹窗」的前提。
 */
export const useWorkspaceStore = defineStore('workspace', () => {
  const states = ref<Record<string, ProjectState>>({})

  function state(name: string): ProjectState {
    let s = states.value[name]
    if (!s) {
      states.value[name] = emptyState(name)
      // 必须回读一次，不能直接 return 刚创建的那个对象：写进 states 的是原始
      // 对象，第一次调用拿到的引用是非响应式的，在它上面改 dirs / expanded
      // 不触发更新。症状是「新建项目后第一次进项目，文件树是空的，刷新一下
      // 才出来」——因为刷新时这次走的是代理。openFile 里的 push 是同一类坑。
      s = states.value[name]
    }
    return s
  }

  /**
   * 丢掉一个项目的全部状态（项目标签关掉时调用，见 ProjectTab 的卸载钩子）。
   *
   * 这份状态是**项目目录在内存里的一份副本**：树、打开过的文件、打包配置、产物……
   * 而项目就躺在磁盘上，应用外面完全可能被动过（换一份 manifest、从别处导进来一份、
   * 删掉再建一个同名的）。留着它的表现就是「关掉再打开，看到的还是上一次那一份」——
   * 树、编辑器里的内容全是旧的，只有手动刷新才更新；换个同名的项目甚至会直接把上一个
   * 项目的文件列出来。
   *
   * 所以关标签 = 退出项目：下次进来走 `ensure` 的冷路径，重新读盘。
   */
  function forget(name: string) {
    delete states.value[name]
  }

  /** 打开项目时做一次完整的初始化。 */
  async function ensure(name: string) {
    const s = state(name)
    if (s.loaded || s.loading) return s
    s.loading = true
    s.error = ''
    try {
      const root = await listDir(name)
      // 返回里没有 entries 说明中间有东西插了一脚（网关/反代的页面、空回包…）。
      // 这种情况必须报出来，否则文件树只会画成「空目录」，看不出是失败了。
      if (!root || !Array.isArray(root.entries)) {
        throw new Error(i18n.global.t('tree.listBroken', { name }))
      }
      s.dirs[name] = root.entries
      s.expanded[name] = true
      // 预先展开 app/ config/ cmd/，这是最常编辑的三个目录。单个目录拉不动
      // 不影响整棵树（展开时会再拉一次，那时候报错更贴切）。
      for (const dir of ['app', 'config', 'cmd']) {
        const child = `${name}/${dir}`
        if (!root.entries.some((e) => e.name === dir && e.dir)) continue
        await loadDir(name, child).catch(() => undefined)
        s.expanded[child] = true
      }
      await Promise.all([reloadPackageInfo(name), reloadShape(name)])
      s.loaded = true
    } catch (e) {
      s.error = errText(e)
    } finally {
      s.loading = false
    }
    return s
  }

  async function loadDir(name: string, path: string, force = false) {
    const s = state(name)
    if (s.dirs[path] && !force) return
    const res = await listDir(path)
    if (!res || !Array.isArray(res.entries)) {
      throw new Error(i18n.global.t('tree.listBroken', { name: path }))
    }
    s.dirs[path] = res.entries
  }

  async function toggleDir(name: string, path: string) {
    const s = state(name)
    const next = !s.expanded[path]
    s.expanded[path] = next
    if (next) {
      try {
        await loadDir(name, path)
      } catch (e) {
        s.error = errText(e)
      }
    }
  }

  async function refreshDir(name: string, path: string) {
    const s = state(name)
    if (s.dirs[path]) await loadDir(name, path, true)
  }

  /**
   * 递归展开所有目录（文件树的「全部展开」）。
   *
   * 逐层拉取：没拉过的目录先 loadDir 才知道下面有什么。符号链接目录不进去
   * （Entry.symlink 为真时 dir 不可信），并用 MAX 兜底，避免病态目录把接口打满。
   */
  async function expandAll(name: string) {
    const s = state(name)
    const MAX_DIRS = 500
    s.expanded[name] = true
    const queue: string[] = [name]
    let visited = 0
    while (queue.length && visited < MAX_DIRS) {
      const dir = queue.shift()!
      visited += 1
      if (!s.dirs[dir]) await loadDir(name, dir).catch(() => undefined)
      for (const entry of s.dirs[dir] ?? []) {
        if (!entry.dir || entry.symlink) continue
        const child = `${dir}/${entry.name}`
        s.expanded[child] = true
        queue.push(child)
      }
    }
  }

  /**
   * 重新拉一遍文件树：已经加载过的目录 + 项目根目录。
   *
   * 根目录一定要带上——树为空（首次加载失败）时 `dirs` 里一个键都没有，
   * 以前这里会什么都不干，「刷新」看着毫无反应。根目录失败记在 state.error 上，
   * 单个子目录失败不阻塞整棵树。
   */
  async function reloadTree(name: string) {
    const s = state(name)
    const paths = Object.keys(s.dirs)
    if (!paths.includes(name)) paths.push(name)
    s.error = ''
    let rootErr: unknown = null
    for (const p of paths) {
      try {
        await loadDir(name, p, true)
      } catch (e) {
        if (p === name) rootErr = e
      }
    }
    await Promise.all([reloadPackageInfo(name), reloadShape(name)])
    if (rootErr) s.error = errText(rootErr)
    else s.loaded = true
  }

  async function reloadPackageInfo(name: string) {
    const s = state(name)
    const [platforms, settings, artifacts] = await Promise.all([
      getPlatforms(name).catch(() => [] as string[]),
      getSettings(name).catch(() => s.settings),
      getArtifacts(name).catch(() => ({ list: [] as Artifact[], outputDir: '' })),
    ])
    s.platforms = platforms
    s.settings = settings
    s.artifacts = artifacts.list
  }

  /** 读一次项目形态（是否 docker、是否带桌面入口）：快捷设置与图标页按它决定可选项。 */
  async function reloadShape(name: string) {
    const s = state(name)
    const shape = await projectExists(name).catch(() => null)
    if (!shape) return
    s.docker = shape.docker
    s.ui = shape.ui
  }

  /** 项目内文件被写入后调用：让图片预览之类的缓存失效。 */
  function bumpAssets(name: string) {
    state(name).assetRev += 1
  }

  // ---- 文件子标签 ----

  function fileTabPath(name: string, path: string) {
    return path.startsWith(`${name}/`) ? path.slice(name.length + 1) : path
  }

  /**
   * 把目录列表缓存（`dirs`）与展开状态（`expanded`）从旧路径搬到新路径，含子树。
   *
   * 两边的键都是「含项目名的完整路径」（`demo/app/server`），所以按前缀搬即可。
   * 目录改名时缓存必须一起走：留在旧键上的那份会在旧名字被重新建出来时冒充新目录的
   * 内容（树上多出已经删掉的子项），而新路径没有展开状态则表现为「改完名就收起来了」。
   */
  function migrateDirKeys(name: string, from: string, to: string) {
    const s = state(name)
    for (const key of Object.keys(s.dirs)) {
      if (key !== from && !key.startsWith(`${from}/`)) continue
      s.dirs[to + key.slice(from.length)] = s.dirs[key]!
      delete s.dirs[key]
    }
    for (const key of Object.keys(s.expanded)) {
      if (key !== from && !key.startsWith(`${from}/`)) continue
      s.expanded[to + key.slice(from.length)] = s.expanded[key]!
      delete s.expanded[key]
    }
  }

  /** 丢掉某个路径下的目录缓存与展开状态（含子树）：目录被删 / 移走之后它们就不成立了。 */
  function dropDirKeys(name: string, path: string) {
    const s = state(name)
    for (const key of Object.keys(s.dirs)) {
      if (key === path || key.startsWith(`${path}/`)) delete s.dirs[key]
    }
    for (const key of Object.keys(s.expanded)) {
      if (key === path || key.startsWith(`${path}/`)) delete s.expanded[key]
    }
  }

  async function openFile(name: string, fullPath: string) {
    const s = state(name)
    const rel = fileTabPath(name, fullPath)
    const existing = s.files.find((f) => f.path === rel)
    if (existing) {
      s.activeTab = rel
      return existing
    }

    // 从固定标签（文件 / 总览 / …）打开文件时记住它，关掉文件时退回这里。
    // 从另一个文件打开（桌面上连着开好几个）不覆盖：那种情况按相邻标签退更自然。
    if (!s.files.some((f) => f.path === s.activeTab)) s.fileReturnTab = s.activeTab

    s.files.push({
      path: rel,
      name: rel.split('/').pop() ?? rel,
      dirty: false,
      loading: true,
      error: '',
      text: true,
      sizeRaw: '',
      origin: '',
      draft: '',
      missing: false,
    })
    s.activeTab = rel

    // 注意：这里必须重新从响应式数组里取一次。push 进去的是原始对象，
    // 直接改局部引用不会触发 Proxy 的更新，界面会一直停在「加载中」。
    const tab = s.files[s.files.length - 1]!
    try {
      const res = await readFile(fullPath)
      tab.text = res.text
      tab.sizeRaw = res.sizeRaw
      tab.origin = res.content
      // 二进制不写 error：是不是「不可编辑」由 EditorHost 按注册表判断
      // （图片有自己的预览器），error 只留真正的失败。
    } catch (e) {
      // 404 = 磁盘上还没有这个文件（向导、生命周期脚本常常是空的）。这不是失败：
      // 按空内容打开，先在内存里编辑，保存时才落盘。
      if (e instanceof ApiError && e.status === 404) {
        tab.missing = true
      } else {
        tab.error = errText(e)
      }
    } finally {
      tab.loading = false
    }
    return tab
  }

  function closeFile(name: string, rel: string) {
    const s = state(name)
    const index = s.files.findIndex((f) => f.path === rel)
    if (index < 0) return
    s.files.splice(index, 1)
    if (s.activeTab !== rel) return

    // 退回「打开这个文件之前停在哪」。那个位置要是已经没了（原来停在另一个文件上、
    // 而它已经被关掉），再退回相邻的文件标签 / 总览。
    const back = s.fileReturnTab
    const alive = back !== '' && back !== rel && (isFixedTab(back) || s.files.some((f) => f.path === back))
    const next = s.files[index] ?? s.files[index - 1]
    s.activeTab = alive ? back : (next ? next.path : DEFAULT_TAB)
  }

  function setActive(name: string, tab: string) {
    state(name).activeTab = tab
  }

  function setDirty(name: string, rel: string, dirty: boolean) {
    const tab = state(name).files.find((f) => f.path === rel)
    if (tab && tab.dirty !== dirty) tab.dirty = dirty
  }

  /** 记下编辑器里还没保存的内容（编辑器被卸载时用，见 FileTab.draft）。 */
  function setDraft(name: string, rel: string, draft: string) {
    const tab = state(name).files.find((f) => f.path === rel)
    if (tab) tab.draft = draft
  }

  /** 从磁盘重新读取某个已打开文件，丢弃未保存的改动。 */
  async function reloadFile(name: string, rel: string) {
    const s = state(name)
    const tab = s.files.find((f) => f.path === rel)
    if (!tab) return
    tab.loading = true
    try {
      const res = await readFile(`${name}/${rel}`)
      tab.text = res.text
      tab.sizeRaw = res.sizeRaw
      tab.origin = res.content
      tab.error = ''
    } catch (e) {
      tab.error = errText(e)
    } finally {
      tab.loading = false
      tab.draft = ''
      tab.dirty = false
    }
  }

  async function saveFile(name: string, rel: string, content: string) {
    const s = state(name)
    const tab = s.files.find((f) => f.path === rel)
    await writeFile(`${name}/${rel}`, content)
    if (tab) {
      // 原来是「磁盘上还不存在」的文件（向导等）：这次保存才真正落盘，
      // 父目录是后端一起建出来的，所以要刷一次树，让新文件出现在左侧。
      if (tab.missing) {
        tab.missing = false
        const parent = rel.includes('/') ? `${name}/${rel.slice(0, rel.lastIndexOf('/'))}` : name
        await refreshDir(name, parent).catch(() => undefined)
      }
      tab.origin = content
      tab.draft = ''
      tab.dirty = false
    }
    if (IMAGE_PATH_RE.test(rel)) bumpAssets(name)
  }

  const dirtyCount = (name: string) => state(name).files.filter((f) => f.dirty).length

  // ---- 文件管理器的形态与位置 ----

  /**
   * 切换文件管理器的形态。
   *
   * 「文件」标签只在标签页模式里存在，所以切回左栏模式时必须把停在上面的面板挪走，
   * 否则面板区会变成一片空白（标签没了，但 activeTab 还指着它）。
   */
  function setViewMode(name: string, mode: ViewMode) {
    const s = state(name)
    s.viewMode = mode
    window.localStorage.setItem(viewModeKey(name), mode)
    if (mode === 'tree') {
      if (s.activeTab === FILES_TAB) s.activeTab = DEFAULT_TAB
      return
    }
    // 切过去就直接停在「文件」标签上：按钮说的是「切到标签页模式」，人还留在总览
    // 的话，看起来就像什么都没发生（只是左边少了一栏）。
    s.activeTab = FILES_TAB
    void loadDir(name, s.browserDir).catch(() => undefined)
  }

  function setBrowserView(name: string, view: BrowserView) {
    state(name).browserView = view
  }

  /** 进入某个目录（压一次后退栈，清掉前进栈——和浏览器一致）。 */
  function browseTo(name: string, dir: string) {
    const s = state(name)
    if (dir === s.browserDir) return
    s.browserBack.push(s.browserDir)
    s.browserForward = []
    s.browserDir = dir
  }

  function browseBack(name: string) {
    const s = state(name)
    const prev = s.browserBack.pop()
    if (prev === undefined) return
    s.browserForward.push(s.browserDir)
    s.browserDir = prev
  }

  function browseForward(name: string) {
    const s = state(name)
    const next = s.browserForward.pop()
    if (next === undefined) return
    s.browserBack.push(s.browserDir)
    s.browserDir = next
  }

  /**
   * 拉标签页模式当前目录的内容。
   *
   * `force` 给「刷新」按钮用：不吃缓存。失败记在 `state.error` 上，面板里就地显示
   * 一句原因和重试——和文件树展开失败的处理保持一致，不要静默画成空目录。
   */
  async function loadBrowserDir(name: string, force = false) {
    const s = state(name)
    s.error = ''
    try {
      await loadDir(name, s.browserDir, force)
    } catch (e) {
      s.error = errText(e)
      return
    }
    if (force) await reloadPackageInfo(name)
  }

  // ---- 剪贴板 ----

  function copyToClipboard(name: string, path: string, cut: boolean) {
    state(name).clipboard = { path, cut }
  }

  function clearClipboard(name: string, path?: string) {
    const s = state(name)
    if (!path || s.clipboard?.path === path) s.clipboard = null
  }

  /**
   * 能不能把剪贴板里的东西粘到 destDir。
   *
   * 除了「剪贴板里有东西」，还要挡住「把目录粘进它自己或者它的子孙里」——
   * 复制会无限递归，剪切会被后端拒绝（`fsx.Rename` 明确挡了这一条）。
   */
  function canPasteTo(name: string, destDir: string): boolean {
    const clip = state(name).clipboard
    if (!clip) return false
    return destDir !== clip.path && !destDir.startsWith(`${clip.path}/`)
  }

  /**
   * 粘贴：复制走 copy，剪切走 rename（同一个存储卷，就是一次改名）。
   *
   * 目标重名时复制会自动换个名字（`xxx - 副本`），绝不覆盖；剪切则直接拒绝——
   * 改名到一个已存在的路径上，轻则报错重则覆盖，不如让用户自己决定。
   *
   * 粘一次就把剪贴板清掉（复制和剪切都是）：剪贴板在这里只是「搬一次文件」的中转，
   * 留着它反而容易在切了几层目录之后粘错地方。
   */
  async function pasteInto(name: string, destDir: string): Promise<PasteOutcome> {
    const s = state(name)
    const clip = s.clipboard
    if (!clip) return 'invalid'
    if (!canPasteTo(name, destDir)) return 'invalid'
    const fileName = clip.path.split('/').pop() ?? ''
    if (!fileName) return 'invalid'

    // 目标目录的内容必须现拉一次（force）：缓存里那份可能还是空的
    // ——「粘贴时刚好同名」的判据全靠它，读旧了就会把别人的文件覆盖掉。
    // 拉不到就干脆不粘：宁可让用户重试，也不冒覆盖的风险。
    try {
      await loadDir(name, destDir, true)
    } catch {
      return 'unreadable'
    }

    const siblings = s.dirs[destDir] ?? []
    const taken = siblings.some((entry) => entry.name === fileName)
    if (taken && clip.cut) return 'exists'

    const to = `${destDir}/${uniqueName(siblings, fileName, i18n.global.t('tree.copySuffix'))}`
    if (clip.cut) await renameEntryAt(name, clip.path, to)
    else await copyEntryAt(name, clip.path, to)
    s.clipboard = null
    // 树里 / 列表里当下就能看到粘贴结果；复制和剪切都只刷了源目录，目标目录补一次。
    s.expanded[destDir] = true
    await refreshDir(name, destDir)
    return 'ok'
  }

  // ---- 文件操作 ----

  async function createEntryAt(name: string, parent: string, fileName: string, dir: boolean) {
    const target = parent.endsWith('/') ? `${parent}${fileName}` : `${parent}/${fileName}`
    // 建文件前必须先确认目标目录里没有同名项。后端 /api/fs/create 也挡了这一条
    // （已存在的文件会被拒绝），但这里现拉一次目录能给出更清楚的提示，也省掉一趟注定
    // 失败的写请求。缓存里那份可能是旧的，所以必须 force 现拉。
    // 目录不查重：「新建目录」是幂等的，重复创建没有破坏性，补架构目录那条路也依赖它。
    if (!dir) {
      try {
        await loadDir(name, parent, true)
      } catch {
        throw new Error(
          i18n.global.t('tree.createListFailed', { name: parent.split('/').pop() ?? parent }),
        )
      }
      if ((state(name).dirs[parent] ?? []).some((entry) => entry.name === fileName)) {
        throw new Error(i18n.global.t('tree.createExists', { name: fileName }))
      }
    }
    await createEntry(target, dir)
    await refreshDir(name, parent)
    if (dir) await refreshDir(name, target)
  }

  async function removeEntryAt(name: string, path: string, dir: boolean) {
    await deleteEntry(path, dir)
    const parent = path.split('/').slice(0, -1).join('/') || name
    await refreshDir(name, parent)
    const s = state(name)
    // 删掉的正好是标签页模式当前所在的目录（或它的祖先）时，游标要退回上一层，
    // 否则那个面板会一直停在一个已经不存在的目录上。
    if (s.browserDir === path || s.browserDir.startsWith(`${path}/`)) {
      s.browserBack = []
      s.browserForward = []
      s.browserDir = parent
    }
    // 缓存与展开状态跟着失效：留着它们的表现是「删掉一个目录再建一个同名的，树上
    // 直接显示上一个目录的子项」。
    dropDirKeys(name, path)
    // 剪贴板里那条（或它里面的东西）已经不存在了，清掉——留着它之后每次粘贴都会失败。
    if (s.clipboard && (s.clipboard.path === path || s.clipboard.path.startsWith(`${path}/`))) {
      s.clipboard = null
    }
    // 关掉被删掉的文件标签
    const rel = fileTabPath(name, path)
    for (const f of [...s.files]) {
      if (f.path === rel || (dir && f.path.startsWith(`${rel}/`))) closeFile(name, f.path)
    }
  }

  /**
   * 重命名文件或目录。
   *
   * 三件事必须一起做对，否则用户会丢内容（都踩过）：
   *
   *  1. **打开着的标签要跟着改路径**。编辑器宿主是按 `file.path` 做 key 的，key 一变
   *     组件就重建，`loadText` 会回落到 `draft || origin`——所以 `origin` / `dirty` /
   *     `draft` 一个都不能在这里清掉，清掉的表现是「改个文件名，编辑框空了，随手敲
   *     一个字保存，文件就只剩那一个字」。内容本身没变，只有名字变了，不需要重读盘。
   *  2. **目录改名要连子标签一起改**。目录里已经打开的文件标签路径还是旧的，保存会
   *     照着旧路径写回一个「复活」的文件（后端写文件会顺手把父目录建出来）。
   *  3. **目标重名要拒绝**。rename(2) 会把同名文件直接覆盖掉，用户只是想改个名，
   *     不该拿另一个文件陪葬。后端 `fsx.Rename` 也挡了这一条，这里先拦是为了给出
   *     一句人话（后端只会说「目标已存在：xxx」）。
   */
  async function renameEntryAt(name: string, path: string, to: string) {
    const s = state(name)
    const fromName = path.split('/').pop() ?? path
    const toName = to.split('/').pop() ?? to
    // 重命名不负责搬家：目标必须还在同一个目录里。名字里带 / 的话后端会把它当成
    // 路径（`fsx.Rel` 会规整成相对路径），文件就被搬进子目录了——用户按「重命名」
    // 的时候想要的不是这个。
    const parent = path.split('/').slice(0, -1).join('/') || name
    const toParent = to.split('/').slice(0, -1).join('/') || name
    if (toParent !== parent || !toName || toName === '.' || toName === '..' || /[\\/]/.test(toName)) {
      throw new Error(i18n.global.t('tree.renameInvalid'))
    }
    if (toName === fromName) return

    // 目标目录的内容必须现拉一次（force）：缓存里那份可能是旧的，拿它判重名会覆盖
    // 别人的文件。拉不到就干脆不改：宁可让用户重试，也不冒覆盖的风险。
    try {
      await loadDir(name, parent, true)
    } catch {
      throw new Error(i18n.global.t('tree.renameListFailed', { name: parent.split('/').pop() ?? parent }))
    }
    if ((s.dirs[parent] ?? []).some((entry) => entry.name === toName)) {
      throw new Error(i18n.global.t('tree.renameExists', { name: toName }))
    }

    await renameEntry(path, to)
    await refreshDir(name, parent)
    // 改名的是一个目录、而标签页模式正待在里面时，游标跟着改过的前缀走。
    if (s.browserDir === path || s.browserDir.startsWith(`${path}/`)) {
      s.browserDir = to + s.browserDir.slice(path.length)
    }
    // 目录列表缓存与展开状态也要一起搬。不搬的表现有两个：展开着的目录改完名变回收起
    // （新路径没有 expanded），以及旧路径那份缓存变成孤儿——旧名字一旦被重新建出来
    // （粘贴 / 解压都可能），树上会画出已被删掉的那批子项。
    migrateDirKeys(name, path, to)
    // 剪贴板里如果是这一项（或它里面的东西），路径要跟着改。不改的话下一次粘贴会拿一个
    // 已经不存在的路径去改名 / 复制，后端报「没有那个文件」；而 pasteInto 只在成功之后
    // 才清剪贴板，于是这个坏路径会一直留着，之后每次粘贴都失败。
    if (s.clipboard) {
      const clip = s.clipboard.path
      if (clip === path) s.clipboard = { ...s.clipboard, path: to }
      else if (clip.startsWith(`${path}/`)) {
        s.clipboard = { ...s.clipboard, path: to + clip.slice(path.length) }
      }
    }

    const rel = fileTabPath(name, path)
    const nextRel = fileTabPath(name, to)
    for (const f of [...s.files]) {
      // 自己，或者自己所在的目录被改名：路径跟着换，内容 / 脏标记 / 草稿原样留着。
      const wasPath = f.path
      if (f.path === rel) {
        f.path = nextRel
      } else if (f.path.startsWith(`${rel}/`)) {
        f.path = nextRel + f.path.slice(rel.length)
      } else {
        continue
      }
      f.name = f.path.split('/').pop() ?? f.path
      // 活动标签也要跟着换，**包括「活动的是被改名目录里的子文件」这一种**：
      // activeTab 是「哪个面板可见」的唯一判据（ProjectTab 里全是
      // v-show="activeTab === …"），只改子标签的 path 会把它留在一个没有任何标签
      // 使用的路径上——表现是「改一下目录名，编辑区整片空白、标签栏也不高亮」。
      if (s.activeTab === wasPath) s.activeTab = f.path
    }
  }

  async function copyEntryAt(name: string, path: string, to: string) {
    await copyEntry(path, to)
    const parent = path.split('/').slice(0, -1).join('/') || name
    await refreshDir(name, parent)
  }

  /**
   * 解压到压缩包所在的目录（后端调系统 tar / gzip）。
   *
   * 同名文件一律跳过，所以返回值里的 `skipped` 要交给调用方告诉用户——不声不响地
   * 少解几个文件，比报错更让人摸不着头脑。
   */
  async function extractAt(name: string, path: string): Promise<ExtractResult> {
    const res = await extractEntry(path)
    // res.dir 是相对**项目根目录**（后端沙箱的根）的，和 `dirs` 的键同一套写法，
    // 直接拿来用；只有解压在项目根上时它才是 "."。
    await refreshDir(name, res.dir === '.' ? name : res.dir)
    bumpAssets(name)
    return res
  }

  /** 上传一批文件到 dir。`onProgress` 收的是 0–100 的百分比，给进度提示用。 */
  async function uploadTo(
    name: string,
    dir: string,
    files: File[],
    overwrite: boolean,
    onProgress?: (percent: number) => void,
  ) {
    const res = await uploadFiles(dir, files, overwrite, onProgress)
    await refreshDir(name, dir)
    bumpAssets(name)
    return res
  }

  const downloadAt = (path: string) => downloadEntry(path)

  // ---- 打包 ----

  async function updateSettings(name: string, patch: Partial<PackSettings>) {
    const s = state(name)
    s.settings = { ...s.settings, ...patch }
    await saveSettings(name, s.settings)
  }

  async function pack(name: string) {
    const s = state(name)
    const logs = useLogsStore()
    s.packing = true
    s.error = ''
    logs.connect()
    try {
      const platforms = s.settings.multiPlatform ? s.settings.platforms : []
      const res = await packProject(name, platforms, s.settings.serverDir, s.settings.copyToOutput)
      s.results = res.results ?? []
      await reloadPackageInfo(name)
      return s.results
    } catch (e) {
      s.error = errText(e)
      throw e
    } finally {
      s.packing = false
    }
  }

  return {
    states,
    state,
    ensure,
    forget,
    loadDir,
    toggleDir,
    refreshDir,
    expandAll,
    reloadTree,
    reloadPackageInfo,
    openFile,
    closeFile,
    setActive,
    setDirty,
    setDraft,
    saveFile,
    reloadFile,
    dirtyCount,
    setViewMode,
    setBrowserView,
    browseTo,
    browseBack,
    browseForward,
    loadBrowserDir,
    copyToClipboard,
    clearClipboard,
    canPasteTo,
    pasteInto,
    createEntryAt,
    removeEntryAt,
    renameEntryAt,
    copyEntryAt,
    extractAt,
    uploadTo,
    downloadAt,
    updateSettings,
    pack,
  }
})
