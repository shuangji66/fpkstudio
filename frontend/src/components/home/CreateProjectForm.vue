<template>
  <div class="panel p-3">
    <div class="mb-2 text-sm font-medium text-slate-700 dark:text-slate-200">{{ t('home.create.title') }}</div>

    <div class="flex flex-wrap items-end gap-4">
      <div>
        <div class="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">{{ t('home.create.name') }}</div>
        <el-input
          v-model="name"
          class="w-64"
          size="default"
          :placeholder="t('home.create.namePlaceholder')"
          @keydown.enter="onEnterKey"
        />
      </div>

      <div>
        <div class="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">{{ t('home.create.kind') }}</div>
        <el-radio-group v-model="docker" size="default">
          <el-radio-button :value="false">{{ t('home.create.native') }}</el-radio-button>
          <el-radio-button :value="true">{{ t('home.create.docker') }}</el-radio-button>
        </el-radio-group>
      </div>

      <div>
        <div class="text-sm font-medium text-slate-500 dark:text-slate-400 mb-1">{{ t('home.create.entry') }}</div>
        <el-radio-group v-model="withUi" size="default">
          <el-radio-button :value="true">{{ t('home.create.withUi') }}</el-radio-button>
          <el-radio-button :value="false">{{ t('home.create.withoutUi') }}</el-radio-button>
        </el-radio-group>
      </div>

      <span class="flex-1" />

      <el-button size="default" @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
      <el-button size="default" type="primary" :loading="loading" @click="submit">
        {{ loading ? t('home.create.creating') : t('home.create.submit') }}
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
import { onEnter } from '@/utils/keyboard'
import type { ProjectInfo } from '@/api/types'

const emit = defineEmits<{
  done: [project: ProjectInfo | null]
  cancel: []
}>()

const { t } = useI18n()
const projects = useProjectsStore()

const name = ref('')
const docker = ref(false)
const withUi = ref(true)
const loading = ref(false)

async function submit() {
  // 回车不看按钮的 loading 态，连按两下会发两次创建请求
  if (loading.value) return
  const value = name.value.trim()
  if (!value) {
    ElMessage.warning(t('home.create.nameRequired'))
    return
  }
  loading.value = true
  try {
    const info = await projects.create(value, docker.value, withUi.value)
    name.value = ''
    emit('done', info)
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : String(e))
  } finally {
    loading.value = false
  }
}

/** 回车 = 创建（组词中的回车不算，见 utils/keyboard.ts）。 */
function onEnterKey(event: Event) {
  onEnter(event, () => void submit())
}
</script>
