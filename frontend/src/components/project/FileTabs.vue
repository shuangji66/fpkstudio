<template>
  <div class="flex shrink-0 items-stretch border-b border-slate-200 bg-slate-50">
    <!--
      三个固定子标签钉在最左边，不参与横向滚动：打开一堆文件之后滚轮只滚文件标签，
      总览 / 快捷设置 / 图标始终在视野里。
    -->
    <div class="flex shrink-0 items-stretch gap-0.5 pl-1">
      <button
        v-for="tab in fixedTabs"
        :key="tab.id"
        type="button"
        class="subtab"
        :class="state.activeTab === tab.id ? 'subtab-active' : 'subtab-idle'"
        @click="workspace.setActive(project, tab.id)"
      >
        <el-icon class="text-[13px]"><component :is="tab.icon" /></el-icon>
        <span>{{ tab.label }}</span>
        <span
          v-if="tab.id === OVERVIEW_TAB && state.packing"
          class="ml-0.5 h-1.5 w-1.5 animate-pulse rounded-full bg-sky-500"
        />
      </button>
    </div>

    <!-- 固定标签与文件标签之间留一条分隔线，避免把两者看成一类 -->
    <span v-if="state.files.length" class="mx-1 w-px shrink-0 self-stretch bg-slate-200" />

    <div
      ref="scroller"
      class="hscroll thin-scroll flex min-w-0 flex-1 items-stretch gap-0.5 pr-1"
      @wheel="wheelToHorizontal"
    >
      <button
        v-for="file in state.files"
        :key="file.path"
        type="button"
        class="subtab group max-w-[16rem]"
        :class="state.activeTab === file.path ? 'subtab-active' : 'subtab-idle'"
        :title="file.path"
        @click="workspace.setActive(project, file.path)"
        @click.middle="close(file.path)"
      >
        <el-icon v-if="file.missing" class="shrink-0 text-[12px] text-emerald-600" :title="t('editor.missing')">
          <DocumentAdd />
        </el-icon>
        <span class="truncate">{{ file.name }}</span>
        <span v-if="file.dirty" class="shrink-0 text-amber-500" :title="t('editor.unsaved')">●</span>
        <el-icon
          class="shrink-0 rounded text-[11px] text-slate-400 opacity-0 hover:bg-slate-200 hover:text-slate-700 group-hover:opacity-100"
          @click.stop="close(file.path)"
        >
          <Close />
        </el-icon>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { DocumentAdd, Operation, Picture, Odometer } from '@element-plus/icons-vue'
import {
  ICON_TAB,
  OVERVIEW_TAB,
  SHORTCUTS_TAB,
  useWorkspaceStore,
} from '@/stores/workspace'
import { revealActive, wheelToHorizontal } from '@/utils/scroll'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))
const scroller = ref<HTMLElement | null>(null)

/**
 * 固定的三个子标签：总览（原打包页）、快捷设置、图标。
 *
 * 顺序固定、不可关闭，文件子标签永远排在它们后面。i18n 是响应式的，所以这里
 * 用 computed 而不是常量数组。
 */
const fixedTabs = computed(() => [
  { id: OVERVIEW_TAB, label: t('overview.title'), icon: Odometer },
  { id: SHORTCUTS_TAB, label: t('shortcuts.title'), icon: Operation },
  { id: ICON_TAB, label: t('iconTab.title'), icon: Picture },
])

// 打开/切换文件时把它滚进可视区，省得手动找
watch(
  () => state.value.activeTab,
  async () => {
    await nextTick()
    revealActive(scroller.value, '.subtab-active')
  },
)

async function close(path: string) {
  const file = state.value.files.find((f) => f.path === path)
  if (file?.dirty) {
    try {
      await ElMessageBox.confirm(
        t('tab.closeDirty', { name: file.name }),
        t('common.tip'),
        {
          type: 'warning',
          confirmButtonText: t('common.confirm'),
          cancelButtonText: t('common.cancel'),
        },
      )
    } catch {
      return
    }
  }
  workspace.closeFile(props.project, path)
}
</script>
