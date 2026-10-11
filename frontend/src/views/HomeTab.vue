<template>
  <div class="flex h-full min-h-0 flex-col">
    <div class="flex-1 overflow-y-auto thin-scroll p-4">
      <div class="mb-3 flex flex-wrap items-center gap-3">
        <div>
          <h1 class="text-lg font-semibold text-slate-800">{{ t('tab.home') }}</h1>
          <p class="mt-0.5 text-xs text-slate-500">
            {{ t('home.subtitle', { count: projects.count }) }}
          </p>
        </div>

        <span class="flex-1" />

        <el-input
          v-model="projects.keyword"
          class="w-56"
          size="small"
          clearable
          :placeholder="t('home.searchPlaceholder')"
          :prefix-icon="Search"
        />
        <el-button size="small" :icon="Refresh" @click="projects.refresh()">
          {{ t('common.refresh') }}
        </el-button>
        <el-button
          size="small"
          :type="panel === 'create' ? 'primary' : 'default'"
          :icon="Plus"
          @click="toggle('create')"
        >
          {{ t('home.newProject') }}
        </el-button>
        <el-button
          size="small"
          :type="panel === 'import' ? 'primary' : 'default'"
          :icon="Upload"
          @click="toggle('import')"
        >
          {{ t('home.importFpk') }}
        </el-button>
      </div>

      <CreateProjectForm
        v-if="panel === 'create'"
        class="mb-3"
        @done="onCreated"
        @cancel="panel = ''"
      />
      <ImportFpkForm v-else-if="panel === 'import'" class="mb-3" @done="onImported" @cancel="panel = ''" />

      <ProjectGrid @open="openProject" @deleted="onDeleted" />

      <details class="panel mt-4 px-3 py-2">
        <summary class="cursor-pointer select-none text-xs font-medium text-slate-600">
          {{ t('system.title') }}
        </summary>
        <div class="mt-2 grid grid-cols-[7rem_1fr] gap-x-3 gap-y-1 text-xs">
          <template v-for="row in infoRows" :key="row.label">
            <div class="text-slate-500">{{ row.label }}</div>
            <div class="mono break-all text-slate-700">{{ row.value }}</div>
          </template>
        </div>
      </details>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, Search, Upload } from '@element-plus/icons-vue'
import CreateProjectForm from '@/components/home/CreateProjectForm.vue'
import ImportFpkForm from '@/components/home/ImportFpkForm.vue'
import ProjectGrid from '@/components/home/ProjectGrid.vue'
import { useProjectsStore } from '@/stores/projects'
import { useSystemStore } from '@/stores/system'
import { useTabsStore } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import type { ProjectInfo } from '@/api/types'

const { t } = useI18n()
const projects = useProjectsStore()
const system = useSystemStore()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()

const panel = ref<'create' | 'import' | ''>('')

const infoRows = computed(() => {
  const info = system.info
  if (!info) return []
  return [
    { label: t('system.version'), value: `fpkstudio ${info.version}` },
    { label: t('system.fnpack'), value: `${info.fnpack.bin} ${info.fnpack.version || ''}`.trim() },
    { label: t('system.dataDir'), value: info.dataDir },
    { label: t('system.projectsDir'), value: info.projectsDir },
    { label: t('system.outputDir'), value: info.outputDir },
    { label: t('system.socket'), value: info.socket },
    { label: t('system.logFile'), value: info.logFile },
    { label: t('system.pid'), value: String(info.pid) },
  ]
})

function toggle(next: 'create' | 'import') {
  panel.value = panel.value === next ? '' : next
}

function openProject(info: ProjectInfo) {
  tabs.openProject(info.name, info.displayName || info.name)
  void workspace.ensure(info.name)
}

function onCreated(info: ProjectInfo | null) {
  panel.value = ''
  if (info) ElMessage.success(t('home.created', { name: info.name }))
}

function onImported(info: ProjectInfo | undefined) {
  panel.value = ''
  if (info) {
    ElMessage.success(t('home.imported', { name: info.name }))
    openProject(info)
  }
}

function onDeleted(name: string) {
  tabs.closeProject(name)
  ElMessage.success(t('home.deleted', { name }))
}
</script>
