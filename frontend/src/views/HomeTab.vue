<template>
  <div class="flex h-full min-h-0 flex-col">
    <!--
      工具条（搜索 + 按钮 + 视图切换）和新建 / 导入面板固定在顶部，不参与下面的滚动：
      列表排布时下方是横向滚动的多列列表，工具条跟着滚出去的话就没法用了。
    -->
    <div class="shrink-0 border-b border-slate-200 dark:border-slate-700 px-4 py-3">
      <div class="flex flex-wrap items-center gap-3">
        <p class="text-xs text-slate-500 dark:text-slate-400">
          {{ t('home.subtitle', { count: projects.count }) }}
        </p>

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

        <span class="flex-1" />

        <!-- 显示切换：平铺（卡片）/ 列表（一行一个），两个独立按钮，记住上一次的选择 -->
        <div class="flex items-center gap-2">
          <el-button
            size="small"
            :type="view === 'grid' ? 'primary' : 'default'"
            :icon="Grid"
            :title="t('home.viewGrid')"
            @click="view = 'grid'"
          >
            {{ t('home.viewGrid') }}
          </el-button>
          <el-button
            size="small"
            :type="view === 'list' ? 'primary' : 'default'"
            :icon="Menu"
            :title="t('home.viewList')"
            @click="view = 'list'"
          >
            {{ t('home.viewList') }}
          </el-button>
        </div>

        <!-- 设置：主题 + 项目根目录 / fpk 输出目录 -->
        <el-button size="small" :icon="Setting" @click="settingsOpen = true">
          {{ t('settings.title') }}
        </el-button>
      </div>

      <CreateProjectForm
        v-if="panel === 'create'"
        class="mt-3"
        @done="onCreated"
        @cancel="panel = ''"
      />
      <ImportFpkForm
        v-else-if="panel === 'import'"
        class="mt-3"
        @done="onImported"
        @cancel="panel = ''"
      />
    </div>

    <div class="min-h-0 flex-1 overflow-auto thin-scroll p-4">
      <ProjectGrid :view="view" @open="openProject" />
    </div>

    <SettingsDialog v-model="settingsOpen" @saved="onSettingsSaved" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { Grid, Menu, Plus, Refresh, Search, Setting, Upload } from '@element-plus/icons-vue'
import CreateProjectForm from '@/components/home/CreateProjectForm.vue'
import ImportFpkForm from '@/components/home/ImportFpkForm.vue'
import ProjectGrid from '@/components/home/ProjectGrid.vue'
import SettingsDialog from '@/components/home/SettingsDialog.vue'
import { useProjectsStore } from '@/stores/projects'
import { useTabsStore } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import { projectLabel } from '@/utils/format'
import type { ProjectInfo } from '@/api/types'

const { t } = useI18n()
const projects = useProjectsStore()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()

const panel = ref<'create' | 'import' | ''>('')
const settingsOpen = ref(false)

/** 显示方式：平铺（卡片）/ 列表（一行一个）。和侧栏宽度一样记在本地。 */
type ViewMode = 'grid' | 'list'
const storedView = window.localStorage.getItem('fpkstudio.homeView')
const view = ref<ViewMode>(storedView === 'list' ? 'list' : 'grid')

watch(view, (next) => window.localStorage.setItem('fpkstudio.homeView', next))

function toggle(next: 'create' | 'import') {
  panel.value = panel.value === next ? '' : next
}

function openProject(info: ProjectInfo) {
  tabs.openProject(info.name, projectLabel(info))
  void workspace.ensure(info.name)
}

/**
 * 目录设置换了之后必须重新拉一次项目列表：项目列表读的就是项目根目录，
 * 还拿着旧目录的那一份，用户会以为设置没生效。
 */
function onSettingsSaved() {
  void projects.refresh()
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
</script>
