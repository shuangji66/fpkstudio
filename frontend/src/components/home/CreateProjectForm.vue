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
    ElMessage.error(createErrorText(e))
  } finally {
    loading.value = false
  }
}

/**
 * fnpack 对项目名的报错只有英文原话（`app name (…) must be a valid name`），
 * 用户看不出到底什么名字算合法。只在「fnpack create 失败 + 名字类错误」后面补一句
 * 本地化规则；权限、磁盘之类的 create 失败不要跟着这条提示，免得误导。
 */
function createErrorText(e: unknown): string {
  const message = e instanceof Error ? e.message : String(e)
  if (
    !message.includes('fnpack create 失败') ||
    !/appname|app name|application name|valid name/i.test(message)
  ) {
    return message
  }
  return `${message}（${t('home.create.nameHint')}）`
}

/** 回车 = 创建（组词中的回车不算，见 utils/keyboard.ts）。 */
function onEnterKey(event: Event) {
  onEnter(event, () => void submit())
}
</script>
