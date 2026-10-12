<template>
  <div class="panel p-3">
    <div class="mb-2 flex items-center gap-2">
      <span class="text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('home.import.title') }}</span>
      <span class="flex-1" />
      <el-button size="default" text @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
    </div>

    <div
      class="flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed px-4 py-6 transition-colors"
      :class="dragging ? 'border-sky-400 bg-sky-50 dark:bg-sky-950/40' : 'border-slate-300 dark:border-slate-600 bg-slate-50 dark:bg-slate-900'"
      @click="pick"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop.prevent="onDrop"
    >
      <el-icon class="text-2xl text-slate-400 dark:text-slate-500"><UploadFilled /></el-icon>
      <p class="text-sm text-slate-600 dark:text-slate-300">
        {{ dragging ? t('home.import.drop') : t('home.import.hint') }}
      </p>
      <div class="flex items-center gap-2">
        <el-button
          size="default"
          :disabled="loading"
          @click.stop="pick"
        >
          {{ t('home.import.pick') }}
        </el-button>
        <!--
          走飞牛的文件选择器：浏览器拿不到 /vol1/... 这种路径，选择器负责把文件
          授权给应用，再由后端直接读它。独立浏览器页面里没有宿主桥，按钮禁用。
        -->
        <el-button
          size="default"
          type="primary"
          plain
          :icon="FolderOpened"
          :disabled="loading || !hostAvailable"
          :title="hostAvailable ? '' : t('trim.error.hostUnavailable')"
          @click.stop="pickHostFile"
        >
          {{ t('home.import.pickHost') }}
        </el-button>
      </div>
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
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { FolderOpened, UploadFilled } from '@element-plus/icons-vue'
import { useProjectsStore } from '@/stores/projects'
import { isHostAvailable, pickUserFiles } from '@/trim/sdk'
import { trimErrorMessage } from '@/trim/errors'
import type { ProjectInfo } from '@/api/types'

const emit = defineEmits<{
  done: [project: ProjectInfo | undefined]
  cancel: []
}>()

const { t, te } = useI18n()
const projects = useProjectsStore()

const input = ref<HTMLInputElement | null>(null)
const dragging = ref(false)
const loading = ref(false)
const percent = ref(0)

const hostAvailable = computed(() => isHostAvailable())

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

/**
 * 「选择飞牛文件」：拉起文件选择器 → 授权给应用 → 由后端按路径导入。
 *
 * 返回的 data 是「当前用户授权给应用的文件」，用最后一个（新选中的那个）。
 * 空数组表示用户没选就关掉了选择器，提示一句取消，不按错误处理。
 */
async function pickHostFile() {
  if (loading.value) return
  loading.value = true
  try {
    const paths = await pickUserFiles(
      t('home.import.pickerTitle'),
      t('home.import.pickerOk'),
      ['.fpk', '.tgz', '.tar'],
    )
    const path = paths[paths.length - 1]
    if (!path) {
      ElMessage.info(t('home.import.canceled'))
      return
    }
    const info = await projects.importFpkPath(path)
    emit('done', info)
  } catch (e) {
    ElMessage.error(trimErrorMessage(t, te, e))
  } finally {
    loading.value = false
  }
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
