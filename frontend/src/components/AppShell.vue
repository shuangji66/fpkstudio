<template>
  <div class="flex h-full flex-col overflow-hidden bg-slate-100 text-slate-800">
    <header
      class="flex shrink-0 items-end gap-3 border-b border-slate-300 bg-slate-200/70 px-2 pt-1.5"
    >
      <div class="flex items-center gap-2 pb-1.5 pl-1.5 pr-2">
        <div
          class="flex h-6 w-6 items-center justify-center rounded-md bg-slate-900 text-[13px] font-bold text-sky-400"
        >
          f
        </div>
        <div class="leading-none">
          <div class="text-sm font-semibold text-slate-800">{{ t('app.name') }}</div>
          <div class="mt-0.5 text-[10px] text-slate-500">v{{ appVersion }}</div>
        </div>
      </div>

      <TabBar class="min-w-0 flex-1" />

      <div class="flex items-center gap-1 pb-1.5">
        <el-tag v-if="system.info && !system.fnpackReady" type="danger" size="small" effect="dark">
          {{ t('system.fnpackMissing') }}
        </el-tag>
        <el-tag v-else-if="system.info" type="info" size="small" effect="plain">
          fnpack {{ system.info.fnpack.version || '?' }}
        </el-tag>
        <el-button text size="small" @click="logs.panelOpen = !logs.panelOpen">
          <el-icon><Tickets /></el-icon>
          <span class="ml-1">{{ t('log.title') }}</span>
          <el-badge v-if="logs.connected" is-dot class="ml-1.5" type="success" />
        </el-button>
      </div>
    </header>

    <main class="relative min-h-0 flex-1 overflow-hidden">
      <HomeTab v-show="tabs.activeId === HOME_TAB_ID" />
      <ProjectTab
        v-for="tab in projectTabs"
        v-show="tabs.activeId === tab.id"
        :key="tab.id"
        :name="tab.project"
      />
    </main>

    <LogConsole />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import HomeTab from '@/views/HomeTab.vue'
import ProjectTab from '@/views/ProjectTab.vue'
import TabBar from '@/components/TabBar.vue'
import LogConsole from '@/components/LogConsole.vue'
import { HOME_TAB_ID, useTabsStore } from '@/stores/tabs'
import { useSystemStore } from '@/stores/system'
import { useProjectsStore } from '@/stores/projects'
import { useLogsStore } from '@/stores/logs'
import { useWorkspaceStore } from '@/stores/workspace'
import { projectPath } from '@/router'
import { appVersion } from '@/runtime'

const { t } = useI18n()
const tabs = useTabsStore()
const system = useSystemStore()
const projects = useProjectsStore()
const logs = useLogsStore()
const workspace = useWorkspaceStore()
const route = useRoute()
const router = useRouter()

const projectTabs = computed(() => tabs.tabs.filter((tab) => tab.kind === 'project'))

/**
 * 路由与标签双向同步。
 *
 * 标签是唯一真相：activate 之后由这里把地址栏改写过去；反过来地址栏变化
 * （刷新、前进后退、外部链接）时再打开并激活对应标签。用 suppress 打断环路。
 */
let suppress = false

watch(
  () => tabs.activeId,
  (id) => {
    const tab = tabs.tabs.find((item) => item.id === id)
    const target = !tab || tab.kind === 'home' ? '/' : projectPath(tab.project)
    if (route.path === target) return
    suppress = true
    void router.replace(target).finally(() => {
      suppress = false
    })
  },
)

watch(
  () => route.fullPath,
  () => {
    if (suppress) return
    const name = typeof route.params.name === 'string' ? route.params.name : ''
    if (route.path.startsWith('/project/') && name) {
      tabs.openProject(name, projects.find(name)?.displayName || name)
      // 支持 #/project/demo?file=manifest 这种直接定位到某个文件的链接
      const file = route.query.file
      if (typeof file === 'string' && file) {
        void workspace.openFile(name, `${name}/${file}`)
      }
    } else {
      tabs.activate(HOME_TAB_ID)
    }
  },
  { immediate: true },
)

/** 项目列表刷新后，把标签标题更新成显示名。 */
watch(
  () => projects.list,
  (list) => {
    for (const tab of projectTabs.value) {
      const info = list.find((p) => p.name === tab.project)
      if (info) tabs.renameProject(tab.project, info.displayName || info.name)
    }
  },
  { deep: true },
)

onMounted(() => {
  void system.load()
  void projects.refresh()
})

onUnmounted(() => {
  logs.disconnect()
})
</script>
