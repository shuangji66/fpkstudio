<template>
  <div class="flex h-full min-h-0 bg-slate-100 dark:bg-slate-900">
    <aside class="flex shrink-0 flex-col border-r border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800" :style="{ width: `${treeWidth}px` }">
      <FileTree :project="name" />
    </aside>

    <div
      class="w-1 shrink-0 cursor-col-resize bg-transparent transition-colors hover:bg-sky-300 dark:hover:bg-sky-700"
      :title="t('editor.resizeHint')"
      @pointerdown="startResize"
    />

    <section class="flex min-w-0 flex-1 flex-col bg-white dark:bg-slate-800">
      <FileTabs :project="name" />

      <div class="relative min-h-0 flex-1">
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

        <EditorHost
          v-for="file in files"
          v-show="activeTab === file.path"
          :key="file.path"
          class="absolute inset-0"
          :project="name"
          :path="file.path"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FileTree from '@/components/project/FileTree.vue'
import FileTabs from '@/components/project/FileTabs.vue'
import OverviewPanel from '@/components/project/OverviewPanel.vue'
import ShortcutsPanel from '@/components/project/ShortcutsPanel.vue'
import IconPanel from '@/components/project/IconPanel.vue'
import EditorHost from '@/components/editor/EditorHost.vue'
import { ICON_TAB, OVERVIEW_TAB, SHORTCUTS_TAB, useWorkspaceStore } from '@/stores/workspace'
import { useProjectsStore } from '@/stores/projects'

const props = defineProps<{ name: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()
const projects = useProjectsStore()

const state = computed(() => workspace.state(props.name))
const files = computed(() => state.value.files)
const activeTab = computed(() => state.value.activeTab)

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
    window.localStorage.setItem('fpkstudio.treeWidth', String(treeWidth.value))
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
}

onMounted(async () => {
  if (!projects.count) await projects.refresh()
  await workspace.ensure(props.name)
})
</script>
