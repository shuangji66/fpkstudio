<template>
  <div class="flex h-full flex-col overflow-hidden bg-slate-100 text-slate-800">
    <header
      class="flex shrink-0 items-end gap-3 border-b border-slate-300 bg-slate-200/70 px-2 pt-1.5"
    >
      <TabBar class="min-w-0 flex-1" />

      <div class="flex items-center gap-1 pb-1.5">
        <el-tag v-if="system.info && !system.fnpackReady" type="danger" size="small" effect="dark">
          {{ t('system.fnpackMissing') }}
        </el-tag>
        <el-button text size="small" :icon="Tickets" @click="logs.dialogOpen = true">
          {{ t('log.title') }}
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

    <LogDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Tickets } from '@element-plus/icons-vue'

import HomeTab from '@/views/HomeTab.vue'
import ProjectTab from '@/views/ProjectTab.vue'
import TabBar from '@/components/TabBar.vue'
import LogDialog from '@/components/LogDialog.vue'
import { HOME_TAB_ID, useTabsStore } from '@/stores/tabs'
import { useSystemStore } from '@/stores/system'
import { useProjectsStore } from '@/stores/projects'
import { useLogsStore } from '@/stores/logs'
import { useWorkspaceStore } from '@/stores/workspace'
import { projectPath } from '@/router'
import { projectLabel } from '@/utils/format'

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
      const info = projects.find(name)
      tabs.openProject(name, info ? projectLabel(info) : name)
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

/** 项目列表刷新后，把标签标题更新成 appname（唯一标识）。 */
watch(
  () => projects.list,
  (list) => {
    for (const tab of projectTabs.value) {
      const info = list.find((p) => p.name === tab.project)
      if (info) tabs.renameProject(tab.project, projectLabel(info))
    }
  },
  { deep: true },
)

onMounted(() => {
  void system.load()
  void projects.refresh()
  // 日志流跟着应用的生命周期走，而不是跟着日志弹窗：弹窗关闭时也要继续收，
  // 这样「已连接」指示灯和未读状态才是准的。
  logs.connect()
})

onUnmounted(() => {
  logs.disconnect()
})
</script>
