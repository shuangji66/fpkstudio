<template>
  <div class="panel p-3">
    <div class="mb-2 text-sm font-medium text-slate-700">{{ t('home.create.title') }}</div>

    <div class="flex flex-wrap items-end gap-4">
      <div>
        <div class="label-text mb-1">{{ t('home.create.name') }}</div>
        <el-input
          v-model="name"
          class="w-64"
          size="small"
          :placeholder="t('home.create.namePlaceholder')"
          @keyup.enter="submit"
        />
      </div>

      <div>
        <div class="label-text mb-1">{{ t('home.create.kind') }}</div>
        <el-radio-group v-model="docker" size="small">
          <el-radio-button :value="false">{{ t('home.create.native') }}</el-radio-button>
          <el-radio-button :value="true">{{ t('home.create.docker') }}</el-radio-button>
        </el-radio-group>
      </div>

      <div>
        <div class="label-text mb-1">{{ t('home.create.entry') }}</div>
        <el-radio-group v-model="withUi" size="small">
          <el-radio-button :value="true">{{ t('home.create.withUi') }}</el-radio-button>
          <el-radio-button :value="false">{{ t('home.create.withoutUi') }}</el-radio-button>
        </el-radio-group>
      </div>

      <span class="flex-1" />

      <el-button size="small" @click="emit('cancel')">{{ t('common.cancel') }}</el-button>
      <el-button size="small" type="primary" :loading="loading" @click="submit">
        {{ loading ? t('home.create.creating') : t('home.create.submit') }}
      </el-button>
    </div>

    <p class="mt-2 text-xs text-slate-500">{{ t('home.create.hint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage } from 'element-plus'
import { useProjectsStore } from '@/stores/projects'
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
</script>
