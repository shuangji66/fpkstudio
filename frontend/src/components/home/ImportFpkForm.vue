<template>
  <div class="panel p-3">
    <div class="mb-2 flex items-center gap-2">
      <span class="text-sm font-medium text-slate-700">{{ t('home.import.title') }}</span>
      <span class="flex-1" />
      <el-button size="small" text @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
    </div>

    <div
      class="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-6 transition-colors"
      :class="dragging ? 'border-sky-400 bg-sky-50' : 'border-slate-300 bg-slate-50'"
      @click="pick"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      <el-icon class="text-2xl text-slate-400"><UploadFilled /></el-icon>
      <p class="text-sm text-slate-600">
        {{ dragging ? t('home.import.drop') : t('home.import.hint') }}
      </p>
      <el-button size="small" :disabled="loading">{{ t('home.import.pick') }}</el-button>
      <input ref="input" type="file" accept=".fpk,.tar,.tgz,application/gzip" class="hidden" @change="onPick" />
    </div>

    <el-progress
      v-if="loading"
      class="mt-3"
      :percentage="percent"
      :stroke-width="6"
      :format="() => t('home.import.uploading', { percent })"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import { useProjectsStore } from '@/stores/projects'
import type { ProjectInfo } from '@/api/types'

const emit = defineEmits<{
  done: [project: ProjectInfo | undefined]
  cancel: []
}>()

const { t } = useI18n()
const projects = useProjectsStore()

const input = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const loading = ref(false)
const percent = ref(0)

function pick() {
  if (!loading.value) input.value?.click()
}

async function onPick(event: Event) {
  const files = (event.target as HTMLInputElement).files
  if (files?.length) await upload(files[0]!)
  if (input.value) input.value.value = ''
}

async function onDrop(event: DragEvent) {
  dragging.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) await upload(file)
}

async function upload(file: File) {
  loading.value = true
  percent.value = 0
  try {
    const info = await projects.importFpk(file, (p) => {
      percent.value = p
    })
    emit('done', info)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
}
</script>
