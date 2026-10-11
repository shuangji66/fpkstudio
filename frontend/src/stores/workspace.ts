import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  copyEntry,
  createEntry,
  deleteEntry,
  downloadEntry,
  getArtifacts,
  getPlatforms,
  getSettings,
  listDir,
  packProject,
  readFile,
  renameEntry,
  saveSettings,
  uploadFiles,
  writeFile,
} from '@/api/endpoints'
import type { Artifact, FsEntry, PackResult, PackSettings } from '@/api/types'
import { useLogsStore } from './logs'

/** 项目标签内固定的「打包」子标签 id。 */
export const PACK_TAB = '__pack__'

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
    activeTab: PACK_TAB,
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
        throw new Error('列目录的返回里没有 entries，可能被中间的网关/反代改写了')
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
      await reloadPackageInfo(name)
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
      throw new Error(`${path} 的列目录返回里没有 entries`)
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
    await reloadPackageInfo(name)
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

  // ---- 文件子标签 ----

  function fileTabPath(name: string, path: string) {
    return path.startsWith(`${name}/`) ? path.slice(name.length + 1) : path
  }

  async function openFile(name: string, fullPath: string) {
    const s = state(name)
    const rel = fileTabPath(name, fullPath)
    const existing = s.files.find((f) => f.path === rel)
    if (existing) {
      s.activeTab = rel
      return existing
    }

    s.files.push({
      path: rel,
      name: rel.split('/').pop() ?? rel,
      dirty: false,
      loading: true,
      error: '',
      text: true,
      sizeRaw: '',
      origin: '',
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
      tab.error = errText(e)
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
    if (s.activeTab === rel) {
      const next = s.files[index] ?? s.files[index - 1]
      s.activeTab = next ? next.path : PACK_TAB
    }
  }

  function setActive(name: string, tab: string) {
    state(name).activeTab = tab
  }

  function setDirty(name: string, rel: string, dirty: boolean) {
    const tab = state(name).files.find((f) => f.path === rel)
    if (tab && tab.dirty !== dirty) tab.dirty = dirty
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
      tab.dirty = false
    }
  }

  async function saveFile(name: string, rel: string, content: string) {
    const s = state(name)
    const tab = s.files.find((f) => f.path === rel)
    await writeFile(`${name}/${rel}`, content)
    if (tab) {
      tab.origin = content
      tab.dirty = false
    }
  }

  const dirtyCount = (name: string) => state(name).files.filter((f) => f.dirty).length

  // ---- 文件操作 ----

  async function createEntryAt(name: string, parent: string, fileName: string, dir: boolean) {
    const target = parent.endsWith('/') ? `${parent}${fileName}` : `${parent}/${fileName}`
    await createEntry(target, dir)
    await refreshDir(name, parent)
    if (dir) await refreshDir(name, target)
  }

  async function removeEntryAt(name: string, path: string, dir: boolean) {
    await deleteEntry(path, dir)
    const parent = path.split('/').slice(0, -1).join('/') || name
    await refreshDir(name, parent)
    // 关掉被删掉的文件标签
    const s = state(name)
    const rel = fileTabPath(name, path)
    for (const f of [...s.files]) {
      if (f.path === rel || (dir && f.path.startsWith(`${rel}/`))) closeFile(name, f.path)
    }
  }

  async function renameEntryAt(name: string, path: string, to: string) {
    await renameEntry(path, to)
    const parent = path.split('/').slice(0, -1).join('/') || name
    await refreshDir(name, parent)
    const s = state(name)
    const rel = fileTabPath(name, path)
    const tab = s.files.find((f) => f.path === rel)
    if (tab) {
      tab.path = fileTabPath(name, to)
      tab.name = tab.path.split('/').pop() ?? tab.path
      tab.origin = ''
      tab.dirty = false
      s.activeTab = tab.path
      await openFile(name, to)
    }
  }

  async function copyEntryAt(name: string, path: string, to: string) {
    await copyEntry(path, to)
    const parent = path.split('/').slice(0, -1).join('/') || name
    await refreshDir(name, parent)
  }

  async function uploadTo(name: string, dir: string, files: File[], overwrite: boolean) {
    const res = await uploadFiles(dir, files, overwrite)
    await refreshDir(name, dir)
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
    saveFile,
    reloadFile,
    dirtyCount,
    createEntryAt,
    removeEntryAt,
    renameEntryAt,
    copyEntryAt,
    uploadTo,
    downloadAt,
    updateSettings,
    pack,
  }
})
