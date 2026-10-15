import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  createProject,
  deleteProject,
  importProject,
  importProjectByPath,
  listProjects,
} from '@/api/endpoints'
import { removePin, renamePin, sortPinned, togglePin } from '@/utils/pins'
import type { ProjectInfo } from '@/api/types'

/** 置顶表在 localStorage 里的键（和主题 / 显示方式一样，是界面偏好不是部署配置）。 */
const PIN_KEY = 'fpkstudio.pinnedProjects'

/**
 * 读回置顶表。
 *
 * 存坏了（手工改过 / 别的版本留下的形状）就当没置顶：这里抛异常的话整个首页都打不开，
 * 而置顶丢了没有任何实际损失。
 */
function loadPins(): string[] {
  try {
    const raw: unknown = JSON.parse(window.localStorage.getItem(PIN_KEY) ?? '[]')
    if (!Array.isArray(raw)) return []
    return raw.filter((name): name is string => typeof name === 'string')
  } catch {
    return []
  }
}

export const useProjectsStore = defineStore('projects', () => {
  const list = ref<ProjectInfo[]>([])
  const loading = ref(false)
  const error = ref('')
  const keyword = ref('')
  /** 置顶的项目名，按「先置顶的在前」排列。 */
  const pinned = ref<string[]>(loadPins())

  const filtered = computed(() => {
    const key = keyword.value.trim().toLowerCase()
    const matched = !key
      ? list.value
      : list.value.filter(
          (p) =>
            p.name.toLowerCase().includes(key) ||
            p.appname.toLowerCase().includes(key) ||
            p.displayName.toLowerCase().includes(key),
        )
    // 置顶的排前面；没置顶的保持后端给的顺序（见 utils/pins.ts）
    return sortPinned(matched, pinned.value)
  })

  const count = computed(() => list.value.length)

  /**
   * 写入置顶表。
   *
   * 浏览器拒绝写 localStorage（隐私模式 / 配额满）不影响这次会话：`pinned` 是内存里的
   * 真值，界面照常反应，只是刷新之后回到上一次成功保存的状态。
   */
  function persistPins(next: string[]) {
    pinned.value = next
    try {
      window.localStorage.setItem(PIN_KEY, JSON.stringify(next))
    } catch {
      // 忽略：置顶写不进去不该弹错误打断用户
    }
  }

  function isPinned(name: string): boolean {
    return pinned.value.includes(name)
  }

  function togglePinByName(name: string) {
    persistPins(togglePin(pinned.value, name))
  }

  /** 项目删掉了，那条置顶跟着删——留着它只会变成一条指向不存在项目的记录。 */
  function forgetPin(name: string) {
    if (pinned.value.includes(name)) persistPins(removePin(pinned.value, name))
  }

  /** 项目改名（改的是目录名）：置顶跟着换名字。 */
  function renamePinned(from: string, to: string) {
    persistPins(renamePin(pinned.value, from, to))
  }

  async function refresh() {
    loading.value = true
    error.value = ''
    try {
      list.value = await listProjects()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function create(name: string, docker: boolean, ui: boolean) {
    const res = await createProject(name, docker, ui)
    await refresh()
    return res.project ?? null
  }

  /**
   * 删除项目：置顶表也要跟着清。
   *
   * 在 `refresh` 之前做——刷完列表调用方就拿不到这个名字了（它已经不在 list 里），
   * 而这个函数是删除唯一收口的地方。
   */
  async function remove(name: string) {
    await deleteProject(name)
    forgetPin(name)
    await refresh()
  }

  async function importFpk(file: File, onProgress?: (p: number) => void, signal?: AbortSignal) {
    const res = await importProject(file, onProgress, signal)
    await refresh()
    return res.project
  }

  /**
   * 导入服务器本机路径上的 fpk（飞牛文件选择器选的）。
   *
   * 文件内容由后端直接读——浏览器拿不到 /vol1/... 这种路径，选择器只负责把
   * 路径授权给应用。
   */
  async function importFpkPath(path: string) {
    const res = await importProjectByPath(path)
    await refresh()
    return res.project
  }

  function find(name: string): ProjectInfo | undefined {
    return list.value.find((p) => p.name === name)
  }

  return {
    list,
    filtered,
    count,
    loading,
    error,
    keyword,
    pinned,
    isPinned,
    togglePin: togglePinByName,
    renamePinned,
    refresh,
    create,
    remove,
    importFpk,
    importFpkPath,
    find,
  }
})
