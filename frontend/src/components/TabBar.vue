<template>
  <div
    ref="scroller"
    class="hscroll thin-scroll flex min-w-0 items-end gap-0.5"
    @wheel="wheelToHorizontal"
  >
    <button
      v-for="tab in tabs.tabs"
      :key="tab.id"
      type="button"
      class="tab-chip group"
      :class="tab.id === tabs.activeId ? 'tab-chip-active' : 'tab-chip-idle'"
      @click="tabs.activate(tab.id)"
      @click.middle="close(tab)"
      @contextmenu.prevent="onContext(tab, $event)"
    >
      <el-icon class="shrink-0 text-[13px]">
        <Folder v-if="tab.kind === 'home'" />
        <Files v-else />
      </el-icon>
      <span class="max-w-[14rem] truncate" :title="tab.title">{{ tab.title }}</span>
      <span
        v-if="tab.kind === 'project' && dirty(tab.project) > 0"
        class="ml-0.5 h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500"
        :title="t('editor.unsaved')"
      />
      <el-icon
        v-if="tab.closable"
        class="shrink-0 rounded text-[12px] text-slate-400 dark:text-slate-500 opacity-0 transition-opacity hover:bg-slate-200 dark:hover:bg-slate-700 hover:text-slate-700 dark:hover:text-slate-200 group-hover:opacity-100"
        @click.stop="close(tab)"
      >
        <Close />
      </el-icon>
    </button>

    <div
      v-if="menu.show"
      class="fixed z-50 rounded-md border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 py-1 text-sm shadow-lg"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
      @click="menu.show = false"
    >
      <button
        type="button"
        class="block w-full whitespace-nowrap px-3 py-1 text-left hover:bg-slate-100 dark:hover:bg-slate-700"
        @click="tabs.closeOthers(menu.id)"
      >
        {{ t('tab.closeOthers') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { nextTick, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useTabsStore, type Tab } from '@/stores/tabs'
import { useWorkspaceStore } from '@/stores/workspace'
import { revealActive, wheelToHorizontal } from '@/utils/scroll'

const { t } = useI18n()
const tabs = useTabsStore()
const workspace = useWorkspaceStore()

const menu = reactive({ show: false, x: 0, y: 0, id: '' })
const scroller = ref<HTMLElement | null>(null)

// 切/开标签时把它滚进可视区，否则新开的项目标签可能落在滚动区外面
watch(
  () => tabs.activeId,
  async () => {
    await nextTick()
    revealActive(scroller.value, '.tab-chip-active')
  },
)

function dirty(project: string): number {
  return project ? workspace.dirtyCount(project) : 0
}

function onContext(tab: Tab, event: MouseEvent) {
  if (!tab.closable) return
  menu.show = true
  menu.x = event.clientX
  menu.y = event.clientY
  menu.id = tab.id
  window.setTimeout(() => window.addEventListener('click', hideMenu, { once: true }), 0)
}

function hideMenu() {
  menu.show = false
}

async function close(tab: Tab) {
  if (!tab.closable) return
  const name = tab.kind === 'project' ? tab.project : ''
  if (name && dirty(name) > 0) {
    try {
      await ElMessageBox.confirm(
        t('tab.closeDirty', { name: tab.title }),
        t('common.tip'),
        { type: 'warning', confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel') },
      )
    } catch {
      return
    }
  }
  tabs.close(tab.id)
}
</script>
