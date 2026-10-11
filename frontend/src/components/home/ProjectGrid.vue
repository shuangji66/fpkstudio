<template>
  <div v-if="projects.loading && !projects.count" class="panel p-8 text-center text-sm text-slate-500">
    {{ t('common.loading') }}
  </div>

  <div v-else-if="!projects.count" class="panel p-8 text-center text-sm text-slate-500">
    {{ t('home.empty') }}
  </div>

  <div v-else-if="!projects.filtered.length" class="panel p-8 text-center text-sm text-slate-500">
    {{ t('home.noMatch') }}
  </div>

  <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(19rem,1fr))] gap-3">
    <article
      v-for="item in projects.filtered"
      :key="item.name"
      class="panel group flex flex-col p-3 transition-shadow hover:shadow-md"
    >
      <div class="flex items-start gap-2">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h3 class="truncate text-sm font-semibold text-slate-800" :title="item.name">
              {{ item.displayName || item.name }}
            </h3>
            <el-tag size="small" :type="item.docker ? 'warning' : 'success'" effect="plain">
              {{ kindLabel(item) }}
            </el-tag>
          </div>
          <p class="mono mt-0.5 truncate text-[11px] text-slate-500" :title="item.appname">
            {{ item.appname || item.name }}
          </p>
        </div>
      </div>

      <dl class="mt-2 grid grid-cols-[3.5rem_1fr] gap-x-2 gap-y-0.5 text-[11px]">
        <dt class="text-slate-400">version</dt>
        <dd class="mono truncate text-slate-600">{{ item.version || '-' }}</dd>
        <dt class="text-slate-400">platform</dt>
        <dd class="mono truncate text-slate-600">{{ item.platform || '-' }}</dd>
        <dt class="text-slate-400">mtime</dt>
        <dd class="truncate text-slate-600">{{ item.modTime }}</dd>
      </dl>

      <div class="mt-3 flex items-center gap-2">
        <el-button size="small" type="primary" @click="emit('open', item)">
          {{ t('home.open') }}
        </el-button>
        <el-button size="small" @click="remove(item)">{{ t('common.delete') }}</el-button>
      </div>
    </article>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
import type { ProjectInfo } from '@/api/types'

const emit = defineEmits<{
  open: [project: ProjectInfo]
  deleted: [name: string]
}>()

const { t } = useI18n()
const projects = useProjectsStore()

function kindLabel(item: ProjectInfo): string {
  if (item.docker) return t('project.kindDocker')
  if (item.ui) return t('project.kindNativeUI')
  return t('project.kindNative')
}

async function remove(item: ProjectInfo) {
  try {
    await ElMessageBox.confirm(
      t('home.confirmDelete', { name: item.displayName || item.name }),
      t('common.tip'),
      { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') },
    )
  } catch {
    return
  }
  try {
    await projects.remove(item.name)
    emit('deleted', item.name)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  }
}
</script>
