import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  createProject,
  deleteProject,
  importProject,
  importProjectByPath,
  listProjects,
} from '@/api/endpoints'
import type { ProjectInfo } from '@/api/types'

export const useProjectsStore = defineStore('projects', () => {
  const list = ref<ProjectInfo[]>([])
  const loading = ref(false)
  const error = ref('')
  const keyword = ref('')

  const filtered = computed(() => {
    const key = keyword.value.trim().toLowerCase()
    if (!key) return list.value
    return list.value.filter(
      (p) =>
        p.name.toLowerCase().includes(key) ||
        p.appname.toLowerCase().includes(key) ||
        p.displayName.toLowerCase().includes(key),
    )
  })

  const count = computed(() => list.value.length)

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

  async function remove(name: string) {
    await deleteProject(name)
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
    refresh,
    create,
    remove,
    importFpk,
    importFpkPath,
    find,
  }
})
