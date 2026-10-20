<template>
  <div class="flex shrink-0 items-stretch border-b border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900">
    <!--
      固定子标签钉在最左边，不参与横向滚动：打开一堆文件之后滚轮只滚文件标签，
      固定标签始终在视野里。
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
        <!-- 移动端没有文件标签栏，开着的文件只藏在这个入口后面，得让它露个点 -->
        <span
          v-else-if="tab.id === FILES_TAB && hiddenDirty"
          class="ml-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500"
          :title="t('editor.unsaved')"
        />
      </button>
    </div>

    <!-- 固定标签与文件标签之间留一条分隔线，避免把两者看成一类 -->
    <span v-if="files.length" class="mx-1 w-px shrink-0 self-stretch bg-slate-200 dark:bg-slate-800" />

    <div
      ref="scroller"
      class="hscroll thin-scroll flex min-w-0 flex-1 items-stretch gap-0.5 pr-1"
      @wheel="wheelToHorizontal"
    >
      <button
        v-for="file in files"
        :key="file.path"
        type="button"
        class="subtab group max-w-[16rem]"
        :class="state.activeTab === file.path ? 'subtab-active' : 'subtab-idle'"
        :title="file.path"
        @click="workspace.setActive(project, file.path)"
        @click.middle="close(file.path)"
      >
        <el-icon v-if="file.missing" class="shrink-0 text-[12px] text-emerald-600 dark:text-emerald-400" :title="t('editor.missing')">
          <DocumentAdd />
        </el-icon>
        <span class="truncate">{{ file.name }}</span>
        <span v-if="file.dirty" class="shrink-0 text-amber-500 dark:text-amber-400" :title="t('editor.unsaved')">●</span>
        <!--
          关闭按钮：红色小叉，桌面鼠标悬停才出现（.hover-reveal 的条件写在 style.css——
          用 Tailwind 的 group-hover 会在触屏平板上永远不显示，那边没有 hover 能力）。
        -->
        <el-icon
          class="close-x hover-reveal text-[11px] transition-opacity"
          :title="t('common.close')"
          :aria-label="t('common.close')"
          @click.stop="close(file.path)"
        >
          <Close />
        </el-icon>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { DocumentAdd, FolderOpened, Operation, Picture, Odometer } from '@element-plus/icons-vue'
import { useCloseFile } from './useCloseFile'
import {
  FILES_TAB,
  ICON_TAB,
  OVERVIEW_TAB,
  SHORTCUTS_TAB,
  useWorkspaceStore,
} from '@/stores/workspace'
import { revealActive, wheelToHorizontal } from '@/utils/scroll'

const props = defineProps<{
  project: string
  /** 移动端：只留固定标签，打开的文件走子页面（见 ProjectTab 的说明）。 */
  mobile?: boolean
}>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const { closeFile } = useCloseFile()

const state = computed(() => workspace.state(props.project))
const scroller = ref<HTMLElement | null>(null)

/** 移动端不渲染文件标签，那些开着的文件只能从这里看出来（从桌面切过去时会碰上）。 */
const files = computed(() => (props.mobile ? [] : state.value.files))
const hiddenDirty = computed(() => props.mobile && state.value.files.some((f) => f.dirty))

/**
 * 固定的子标签：文件、总览（原打包页）、快捷设置、图标。
 *
 * 顺序固定、不可关闭，文件子标签永远排在它们后面。i18n 是响应式的，所以这里
 * 用 computed 而不是常量数组。
 *
 * 「文件」只在标签页模式（或移动端，那边固定就是这个形态）里出现：左栏文件树
 * 模式下文件树就长在侧边栏里，再给一个同名标签是重复的。
 */
const fixedTabs = computed<{ id: string; label: string; icon: Component }[]>(() => {
  const tabs: { id: string; label: string; icon: Component }[] = [
    { id: OVERVIEW_TAB, label: t('overview.title'), icon: Odometer },
    { id: SHORTCUTS_TAB, label: t('shortcuts.title'), icon: Operation },
    { id: ICON_TAB, label: t('iconTab.title'), icon: Picture },
  ]
  if (props.mobile || state.value.viewMode === 'tabs') {
    tabs.unshift({ id: FILES_TAB, label: t('files.title'), icon: FolderOpened })
  }
  return tabs
})

// 打开/切换文件时把它滚进可视区，省得手动找
watch(
  () => state.value.activeTab,
  async () => {
    await nextTick()
    revealActive(scroller.value, '.subtab-active')
  },
)

// 桌面上关文件子标签和关项目标签一样停在左边那个；移动端的标签栏不渲染文件，
// 这条路径只服务桌面（mobile 时 files 是空数组）。
const close = (path: string) => void closeFile(props.project, path, { adjacent: true })
</script>
