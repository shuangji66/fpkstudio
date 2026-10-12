<template>
  <div class="relative flex h-full min-h-0 bg-slate-100 dark:bg-slate-900">
    <aside
      v-if="showTree"
      class="flex shrink-0 flex-col border-r border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800"
      :style="{ width: `${treeWidth}px` }"
    >
      <FileTree :project="name" />
    </aside>

    <div
      v-if="showTree"
      class="w-1 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-sky-300 dark:hover:bg-sky-700"
      :title="t('editor.resizeHint')"
      @pointerdown="startResize"
    />

    <section class="flex min-w-0 flex-1 flex-col bg-white dark:bg-slate-800">
      <FileTabs :project="name" :mobile="mobile" />

      <div class="relative min-h-0 flex-1">
        <FileBrowser
          v-show="activeTab === FILES_TAB"
          :project="name"
          :mobile="mobile"
          class="absolute inset-0"
        />
        <OverviewPanel
          v-show="activeTab === OVERVIEW_TAB"
          :project="name"
          class="absolute inset-0"
        />
        <ShortcutsPanel
          v-show="activeTab === SHORTCUTS_TAB"
          :project="name"
          class="absolute inset-0"
        />
        <IconPanel v-show="activeTab === ICON_TAB" :project="name" class="absolute inset-0" />

        <!--
          桌面：打开的文件就是子标签，全部保持挂载，切标签不丢未保存的内容。
          移动端换一种呈现（见下面的移动端子页面），但同样是全部挂载。
        -->
        <template v-if="!mobile">
          <EditorHost
            v-for="file in files"
            v-show="activeTab === file.path"
            :key="file.path"
            class="absolute inset-0"
            :project="name"
            :path="file.path"
          />
        </template>
      </div>
    </section>

    <!--
      移动端的文件子页面：盖住整个项目区（含子标签栏），编辑器工具栏最左边那个后退
      等同于关掉这个文件标签——未保存的提醒走的是同一段逻辑（useCloseFile）。
    -->
    <template v-if="mobile">
      <EditorHost
        v-for="file in files"
        v-show="activeTab === file.path"
        :key="file.path"
        class="absolute inset-0 z-10"
        :project="name"
        :path="file.path"
        :back="() => closeFile(name, file.path)"
      />
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FileTree from '@/components/project/FileTree.vue'
import FileTabs from '@/components/project/FileTabs.vue'
import FileBrowser from '@/components/project/FileBrowser.vue'
import OverviewPanel from '@/components/project/OverviewPanel.vue'
import ShortcutsPanel from '@/components/project/ShortcutsPanel.vue'
import IconPanel from '@/components/project/IconPanel.vue'
import EditorHost from '@/components/editor/EditorHost.vue'
import { useCloseFile } from '@/components/project/useCloseFile'
import { FILES_TAB, ICON_TAB, OVERVIEW_TAB, SHORTCUTS_TAB, useWorkspaceStore } from '@/stores/workspace'
import { useProjectsStore } from '@/stores/projects'

const props = defineProps<{
  name: string
  /** 移动端形态：固定标签页模式、不显示文件树、文件走子页面。 */
  mobile?: boolean
}>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const projects = useProjectsStore()
const { closeFile } = useCloseFile()

const state = computed(() => workspace.state(props.name))
const files = computed(() => state.value.files)
const activeTab = computed(() => state.value.activeTab)

/** 左栏文件树只在「左栏模式」下占地方；移动端永远没有它。 */
const showTree = computed(() => !props.mobile && state.value.viewMode === 'tree')

const stored = window.localStorage.getItem('fpkstudio.treeWidth')
const treeWidth = ref(stored ? Number(stored) : 300)

function startResize(event: PointerEvent) {
  const startX = event.clientX
  const startWidth = treeWidth.value
  const move = (e: PointerEvent) => {
    treeWidth.value = Math.min(Math.max(startWidth + (e.clientX - startX), 180), 640)
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    // pointercancel 也要摘：触屏手势被系统接管、或者拖拽被取消时不会再来 pointerup，
    // 这两个监听就留在窗口上了——之后鼠标一动树就跟着缩，直到下一次松手。
    window.removeEventListener('pointercancel', up)
    window.localStorage.setItem('fpkstudio.treeWidth', String(treeWidth.value))
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', up)
}

onMounted(async () => {
  if (!projects.count) await projects.refresh()
  await workspace.ensure(props.name)
})
</script>
