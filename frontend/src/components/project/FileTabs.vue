<template>
  <div class="flex shrink-0 items-stretch gap-0.5 overflow-x-auto thin-scroll border-b border-slate-200 bg-slate-50 px-1">
    <button
      type="button"
      class="subtab"
      :class="state.activeTab === PACK_TAB ? 'subtab-active' : 'subtab-idle'"
      @click="workspace.setActive(project, PACK_TAB)"
    >
      <el-icon class="text-[13px]"><Box /></el-icon>
      <span>{{ t('pack.title') }}</span>
      <span
        v-if="state.packing"
        class="ml-0.5 h-1.5 w-1.5 animate-pulse rounded-full bg-sky-500"
      />
    </button>

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
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessageBox } from 'element-plus'
import { useWorkspaceStore, PACK_TAB } from '@/stores/workspace'

const props = defineProps<{ project: string }>()

const { t } = useI18n()
const workspace = useWorkspaceStore()

const state = computed(() => workspace.state(props.project))

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
