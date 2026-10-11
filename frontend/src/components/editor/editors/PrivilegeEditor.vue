<template>
  <div class="h-full min-h-0 overflow-y-auto thin-scroll px-4 py-3">
    <div class="mx-auto max-w-2xl">
      <div v-if="model.error" class="mb-3 rounded-md bg-red-50 px-3 py-2 text-xs text-red-700">
        {{ t('editor.readFailed') }}: {{ model.error }}
      </div>

      <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-2">
        <span class="text-right text-xs text-slate-600">{{ t('privilege.runAs') }}</span>
        <el-select v-model="model.root.defaults['run-as']" size="small" class="w-64">
          <el-option :label="t('privilege.package')" value="package" />
          <el-option :label="t('privilege.root')" value="root" />
        </el-select>
      </div>

      <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-2">
        <span class="text-right text-xs text-slate-600">{{ t('privilege.username') }}</span>
        <el-input v-model="model.root.username" size="small" class="w-64" placeholder="（按 appname 生成）" />
      </div>

      <div class="grid grid-cols-[10rem_1fr] items-center gap-x-3 py-2">
        <span class="text-right text-xs text-slate-600">{{ t('privilege.groupname') }}</span>
        <el-input v-model="model.root.groupname" size="small" class="w-64" placeholder="（按 appname 生成）" />
      </div>

      <div class="grid grid-cols-[10rem_1fr] items-start gap-x-3 py-2">
        <span class="pt-1.5 text-right text-xs text-slate-600">{{ t('privilege.joinGroups') }}</span>
        <el-select
          v-model="model.root['join-groups']"
          size="small"
          multiple
          filterable
          allow-create
          default-first-option
          class="w-64"
          placeholder="video / render"
        >
          <el-option v-for="item in model.root['join-groups']" :key="item" :label="item" :value="item" />
        </el-select>
      </div>

      <p class="mt-3 rounded-md bg-slate-50 px-3 py-2 text-xs leading-relaxed text-slate-600">
        {{ t('privilege.hint') }}
      </p>
      <p
        v-if="model.root.defaults['run-as'] === 'root'"
        class="mt-2 flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs leading-relaxed text-amber-800"
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

const { model } = useEditor<Model>(
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
    // 键顺序固定成 defaults → username → groupname → join-groups → 其它，
    // 避免每次保存都产生无意义的 diff。
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
</script>
