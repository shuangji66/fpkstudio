import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { i18n } from '@/i18n'

/**
 * 主标签模型。
 *
 * 首页（项目管理器）是固定标签、不可关闭；每个打开的项目占一个标签。
 * 标签状态是本应用导航的唯一真相，路由只是它的投影（见 router/index.ts）。
 */
export interface TabBase {
  id: string
  title: string
  closable: boolean
}

export interface HomeTab extends TabBase {
  kind: 'home'
}

export interface ProjectTab extends TabBase {
  kind: 'project'
  project: string
}

export type Tab = HomeTab | ProjectTab

export const HOME_TAB_ID = 'home'

export const useTabsStore = defineStore('tabs', () => {
  // 首页标签的标题走 i18n，别再硬编码（以前写死「项目管理器」，英文界面也是中文）
  const tabs = ref<Tab[]>([
    { id: HOME_TAB_ID, kind: 'home', title: i18n.global.t('tab.home'), closable: false },
  ])
  const activeId = ref<string>(HOME_TAB_ID)

  const active = computed(() => tabs.value.find((t) => t.id === activeId.value) ?? null)
  const activeProject = computed(() =>
    active.value?.kind === 'project' ? active.value.project : '',
  )

  function projectTabId(name: string): string {
    return `project:${name}`
  }

  function activate(id: string) {
    if (tabs.value.some((t) => t.id === id)) activeId.value = id
  }

  function openProject(name: string, title?: string) {
    const id = projectTabId(name)
    const existing = tabs.value.find((t) => t.id === id)
    if (existing) {
      if (title && existing.title !== title) existing.title = title
    } else {
      tabs.value.push({
        id,
        kind: 'project',
        project: name,
        title: title || name,
        closable: true,
      })
    }
    activeId.value = id
  }

  function renameProject(name: string, title: string) {
    const tab = tabs.value.find((t) => t.id === projectTabId(name))
    if (tab) tab.title = title
  }

  function close(id: string) {
    const index = tabs.value.findIndex((t) => t.id === id)
    if (index < 0 || !tabs.value[index]!.closable) return
    const wasActive = activeId.value === id
    tabs.value.splice(index, 1)
    if (wasActive) {
      const next = tabs.value[index] ?? tabs.value[index - 1] ?? tabs.value[0]!
      activeId.value = next.id
    }
  }

  /** 关闭项目标签（项目被删除时用）。 */
  function closeProject(name: string) {
    close(projectTabId(name))
  }

  function closeOthers(id: string) {
    tabs.value = tabs.value.filter((t) => !t.closable || t.id === id)
    activeId.value = id
  }

  return {
    tabs,
    activeId,
    active,
    activeProject,
    activate,
    openProject,
    renameProject,
    close,
    closeProject,
    closeOthers,
    projectTabId,
  }
})
