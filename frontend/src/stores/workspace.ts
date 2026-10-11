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
      s = emptyState(name)
      states.value[name] = s
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
      const [root] = await Promise.all([listDir(`${name}`)])
      s.dirs[name] = root.entries
      s.expanded[name] = true
      if (!s.dirs[`${name}/app`]) {
        // 预先展开 app/ 与 config/，这是最常编辑的两个目录。
        for (const dir of ['app', 'config', 'cmd']) {
          if (root.entries.some((e) => e.name === dir && e.dir)) {
            await loadDir(name, `${name}/${dir}`)
            s.expanded[`${name}/${dir}`] = true
          }
        }
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

  async function reloadTree(name: string) {
    const s = state(name)
    const paths = Object.keys(s.dirs)
    await Promise.all(paths.map((p) => loadDir(name, p, true).catch(() => undefined)))
    await reloadPackageInfo(name)
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
      if (!res.text) tab.error = '这是二进制文件，无法以文本方式编辑'
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
      tab.error = res.text ? '' : '这是二进制文件，无法以文本方式编辑'
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
