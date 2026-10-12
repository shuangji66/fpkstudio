<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-2xl">
      <div v-if="model.error" class="mb-3 rounded-md bg-red-50 dark:bg-red-950/40 px-3 py-2 text-xs text-red-700 dark:text-red-300">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <!--
        和 manifest / 入口配置同一套排法：标题一行、控件一行、字号 14px。
        桌面和窄屏是同一种排布，所以这里不挂 form-stack——那是给「桌面两列、窄屏竖排」
        的表单用的，这份表单本来就是竖排。
      -->
      <div class="form-row flex flex-col gap-1 py-2">
        <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('privilege.runAs') }}</span>
        <el-select v-model="model.root.defaults['run-as']" size="default">
          <el-option :label="t('privilege.package')" value="package" />
          <el-option :label="t('privilege.root')" value="root" />
        </el-select>
      </div>

      <div class="form-row flex flex-col gap-1 py-2">
        <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('privilege.username') }}</span>
        <el-input
          v-model="model.root.username"
          size="default"
          :placeholder="t('privilege.usernamePlaceholder')"
        />
      </div>

      <div class="form-row flex flex-col gap-1 py-2">
        <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('privilege.groupname') }}</span>
        <el-input
          v-model="model.root.groupname"
          size="default"
          :placeholder="t('privilege.groupnamePlaceholder')"
        />
      </div>

      <div class="form-row flex flex-col gap-1 py-2">
        <span class="form-label text-sm text-slate-600 dark:text-slate-300">{{ t('privilege.joinGroups') }}</span>
        <el-select
          v-model="model.root['join-groups']"
          size="default"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="video / render"
        >
          <el-option v-for="item in model.root['join-groups']" :key="item" :label="item" :value="item" />
        </el-select>
      </div>

      <p class="mt-3 rounded-md bg-slate-50 dark:bg-slate-900 px-3 py-2 text-sm leading-relaxed text-slate-600 dark:text-slate-300">
        {{ t('privilege.hint') }}
      </p>
      <p
        v-if="model.root.defaults['run-as'] === 'root'"
        class="mt-2 flex items-start gap-2 rounded-md bg-amber-50 dark:bg-amber-950/40 px-3 py-2 text-sm leading-relaxed text-amber-800 dark:text-amber-200"
      >
        <el-icon class="mt-0.5 shrink-0"><WarningFilled /></el-icon>
        {{ t('privilege.rootWarning') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { WarningFilled } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { parseJSON, stringifyOrdered, useEditor, type EditorProps } from '../useEditor'

interface PrivRoot {
  defaults: Record<string, unknown>
  username?: string
  groupname?: string
  'join-groups'?: string[]
  [key: string]: unknown
}

interface Model {
  root: PrivRoot
  error: string
}

const props = defineProps<EditorProps>()
const emit = defineEmits<{ dirty: [value: boolean] }>()
const { t } = useI18n()

const { model, api } = useEditor<Model>(
  props,
  emit,
  (text) => {
    const parsed = parseJSON<Record<string, unknown>>(text, {})
    const root = (parsed.value ?? {}) as PrivRoot
    if (!root.defaults || typeof root.defaults !== 'object' || Array.isArray(root.defaults)) {
      root.defaults = { 'run-as': 'package' }
    }
    if (typeof root.defaults['run-as'] !== 'string') root.defaults['run-as'] = 'package'
    if (!Array.isArray(root['join-groups'])) root['join-groups'] = []
    if (typeof root.username !== 'string') root.username = ''
    if (typeof root.groupname !== 'string') root.groupname = ''
    return { root, error: parsed.error }
  },
  (value) => {
    const root = value.root
    const out: Record<string, unknown> = { defaults: { ...root.defaults } }
    // 键序最终由 stringifyOrdered 统一按**字母序**输出（不是这里的插入顺序），
    // 目的同样是「每次保存都产出同一份文本」，避免无意义的 diff。
    if (root.username) out.username = root.username
    if (root.groupname) out.groupname = root.groupname
    if (root['join-groups']?.length) out['join-groups'] = [...root['join-groups']]
    for (const [key, raw] of Object.entries(root)) {
      if (key in out || key === 'defaults' || key === 'username' || key === 'groupname') continue
      if (key === 'join-groups') continue
      out[key] = raw
    }
    if (!Object.keys(out.defaults as object).length) {
      out.defaults = { 'run-as': 'package' }
    }
    return stringifyOrdered(out)
  },
)

defineExpose({
  ...api,
  /**
   * 解析失败时拦住保存。
   *
   * 解析失败 = 表单里那份模型是空的，`getContent()` 交出去的是一份空配置。直接保存
   * 就会把用户那份（也许只是少了个括号、还能救的）文件覆盖掉——不可逆，所以宁可
   * 不让存：在源码视图里改回来再存。
   */
  saveBlocked: () =>
    model.value.error ? t('editor.formatJson', { detail: model.value.error }) : '',
})
</script>
